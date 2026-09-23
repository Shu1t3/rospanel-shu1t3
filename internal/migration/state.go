package migration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MigrationPhase describes the current stage of master node migration.
type MigrationPhase string

const (
	PhaseIdle           MigrationPhase = "idle"
	PhasePrepare        MigrationPhase = "prepare"
	PhaseCopying        MigrationPhase = "copying"
	PhaseCandidateReady MigrationPhase = "candidate_ready"
	PhaseSwitching      MigrationPhase = "switching"
	PhaseStandby        MigrationPhase = "standby"
	PhaseCompleted      MigrationPhase = "completed"
	PhaseError          MigrationPhase = "error"
	PhaseRolledBack     MigrationPhase = "rolled_back"
)

// ServerRole describes the operating mode of this RosPanel instance.
type ServerRole string

const (
	RoleMaster         ServerRole = "master"
	RoleCandidate      ServerRole = "candidate"
	RoleStandby        ServerRole = "standby"
	RoleDecommissioned ServerRole = "decommissioned"
)

// PairTokenLifetime is the duration for which candidate pairing token is valid.
const PairTokenLifetime = 15 * time.Minute

// CheckResult records the status of one pre-switch verification check.
type CheckResult struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Required bool   `json:"required"`
	Details  string `json:"details"`
	Error    string `json:"error,omitempty"`
}

// StandbyStats reports metrics while the old master is in standby mode.
type StandbyStats struct {
	ActiveClients      int   `json:"active_clients"`
	LastSeenClientAt   int64 `json:"last_seen_client_at"` // unix
	SwitchedAt         int64 `json:"switched_at"`         // unix
	SyncFailures       int   `json:"sync_failures"`
	LastSyncAt         int64 `json:"last_sync_at"`
	TrafficUpStandby   int64 `json:"traffic_up_standby"`
	TrafficDownStandby int64 `json:"traffic_down_standby"`
}

// Session holds the complete state of a master migration attempt.
type Session struct {
	ID                 string         `json:"id"`
	Phase              MigrationPhase `json:"phase"`
	Role               ServerRole     `json:"role"`
	PublicDomain       string         `json:"public_domain"`
	CandidateAddr      string         `json:"candidate_addr"`
	PairToken          string         `json:"-"` // sensitive: never serialized to clients
	PairTokenExpiresAt int64          `json:"pair_token_expires_at"`
	CreatedAt          int64          `json:"created_at"`
	UpdatedAt          int64          `json:"updated_at"`
	Fenced             bool           `json:"fenced"`
	DNSType            string         `json:"dns_type"` // "cloudflare", "manual"
	DNSRecordID        string         `json:"dns_record_id,omitempty"`
	DNSZoneID          string         `json:"dns_zone_id,omitempty"`
	OriginalTTL        int            `json:"original_ttl,omitempty"`
	CandidatePublicIP  string         `json:"candidate_public_ip,omitempty"`
	OldMasterIP        string         `json:"old_master_ip,omitempty"`
	Checks             []CheckResult  `json:"checks,omitempty"`
	Standby            StandbyStats   `json:"standby,omitempty"`
	LastError          string         `json:"last_error,omitempty"`
	DisasterRecovery   bool           `json:"disaster_recovery,omitempty"`
}

// StateManager persists and governs migration state transitions ensuring only
// one active migration and single active master.
type StateManager struct {
	mu      sync.RWMutex
	dataDir string
	session *Session
}

const stateFileName = "migration_state.json"

// NewStateManager loads existing migration state or starts with an idle session.
func NewStateManager(dataDir string) (*StateManager, error) {
	sm := &StateManager{dataDir: dataDir}
	if err := sm.load(); err != nil {
		return nil, err
	}
	return sm, nil
}

func (sm *StateManager) filePath() string {
	return filepath.Join(sm.dataDir, stateFileName)
}

func (sm *StateManager) load() error {
	path := sm.filePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			sm.session = &Session{
				Phase: PhaseIdle,
				Role:  RoleMaster,
			}
			return nil
		}
		return err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return fmt.Errorf("corrupt migration state: %w", err)
	}
	sm.session = &sess
	return nil
}

func (sm *StateManager) saveLocked() error {
	if sm.session == nil {
		return nil
	}
	sm.session.UpdatedAt = time.Now().Unix()
	data, err := json.MarshalIndent(sm.session, "", "  ")
	if err != nil {
		return err
	}
	tmp := sm.filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, sm.filePath())
}

// GetSession returns a copy of the current migration session.
func (sm *StateManager) GetSession() Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.session == nil {
		return Session{Phase: PhaseIdle, Role: RoleMaster}
	}
	return *sm.session
}

// IsFenced reports whether mutations must be rejected.
func (sm *StateManager) IsFenced() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.session != nil && sm.session.Fenced
}

// StartMigration initiates a new migration to candidateAddr. Rejects if a migration is already active.
func (sm *StateManager) StartMigration(publicDomain, candidateAddr, dnsType string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session != nil && (sm.session.Phase != PhaseIdle && sm.session.Phase != PhaseCompleted && sm.session.Phase != PhaseRolledBack && sm.session.Phase != PhaseError) {
		return "", errors.New("миграция уже активна, завершите или откатите текущую перед началом новой")
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	pairToken := hex.EncodeToString(tokenBytes)

	now := time.Now()
	sm.session = &Session{
		ID:                 fmt.Sprintf("mig-%d", now.UnixNano()),
		Phase:              PhasePrepare,
		Role:               RoleMaster,
		PublicDomain:       publicDomain,
		CandidateAddr:      candidateAddr,
		PairToken:          pairToken,
		PairTokenExpiresAt: now.Add(PairTokenLifetime).Unix(),
		CreatedAt:          now.Unix(),
		UpdatedAt:          now.Unix(),
		Fenced:             false,
		DNSType:            dnsType,
		Checks:             nil,
	}

	if err := sm.saveLocked(); err != nil {
		return "", err
	}
	return pairToken, nil
}

// ValidatePairToken validates the one-time pair token and marks it consumed.
func (sm *StateManager) ValidatePairToken(token string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil || sm.session.PairToken == "" {
		return false
	}
	if time.Now().Unix() > sm.session.PairTokenExpiresAt {
		return false
	}
	if sm.session.PairToken != token {
		return false
	}
	// Invalidate pair token after successful use
	sm.session.PairToken = ""
	_ = sm.saveLocked()
	return true
}

// SetPhase transitions the migration to a new phase safely.
func (sm *StateManager) SetPhase(phase MigrationPhase) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		return errors.New("no active migration session")
	}
	sm.session.Phase = phase
	return sm.saveLocked()
}

// SetFenced enables or disables mutation fencing.
func (sm *StateManager) SetFenced(fenced bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		return errors.New("no active migration session")
	}
	sm.session.Fenced = fenced
	return sm.saveLocked()
}

// SetRole transitions server role (e.g. Master -> Standby, or Candidate -> Master).
func (sm *StateManager) SetRole(role ServerRole) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		sm.session = &Session{Phase: PhaseIdle}
	}
	sm.session.Role = role
	return sm.saveLocked()
}

// UpdateChecks stores verification results.
func (sm *StateManager) UpdateChecks(checks []CheckResult) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		return errors.New("no active migration session")
	}
	sm.session.Checks = checks
	return sm.saveLocked()
}

// SetError records a migration failure.
func (sm *StateManager) SetError(errStr string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		sm.session = &Session{Phase: PhaseIdle, Role: RoleMaster}
	}
	sm.session.Phase = PhaseError
	sm.session.LastError = errStr
	sm.session.Fenced = false
	return sm.saveLocked()
}

// Rollback cancels migration and clears fencing.
func (sm *StateManager) Rollback() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		return nil
	}
	sm.session.Phase = PhaseRolledBack
	sm.session.Fenced = false
	sm.session.PairToken = ""
	return sm.saveLocked()
}

// Complete marks the migration finished and cleans up temporary credentials.
func (sm *StateManager) Complete() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		return nil
	}
	sm.session.Phase = PhaseCompleted
	sm.session.Fenced = false
	sm.session.PairToken = ""
	return sm.saveLocked()
}

// UpdateStandby updates statistics for standby mode.
func (sm *StateManager) UpdateStandby(fn func(*StandbyStats)) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session == nil {
		return errors.New("no active migration session")
	}
	fn(&sm.session.Standby)
	return sm.saveLocked()
}
