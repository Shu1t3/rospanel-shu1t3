package migration

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// StoreReader provides read access to the local database state needed for migration.
type StoreReader interface {
	GetSettings() (*model.Settings, error)
	ListUsers() ([]model.User, error)
	ListNodes() ([]model.Node, error)
	Checkpoint() error
}

// Coordinator orchestrates the migration lifecycle across both old master and candidate nodes.
type Coordinator struct {
	mu           sync.Mutex
	dataDir      string
	stateManager *StateManager
	store        StoreReader
	checker      *PreflightChecker
	httpClient   *http.Client
}

// NewCoordinator initializes the migration coordinator.
func NewCoordinator(dataDir string, store StoreReader) (*Coordinator, error) {
	sm, err := NewStateManager(dataDir)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &Coordinator{
		dataDir:      dataDir,
		stateManager: sm,
		store:        store,
		checker:      NewPreflightChecker(),
		httpClient:   &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}, nil
}

// StateManager returns the underlying state manager.
func (c *Coordinator) StateManager() *StateManager {
	return c.stateManager
}

// InitiateMigration begins migration by recording candidate technical address and generating pairing token.
func (c *Coordinator) InitiateMigration(candidateAddr, dnsType string, cfConfig *CloudflareConfig) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	set, err := c.store.GetSettings()
	if err != nil {
		return "", fmt.Errorf("load settings: %w", err)
	}

	cleanCand := strings.TrimSpace(candidateAddr)
	if cleanCand == "" {
		return "", errors.New("технический адрес кандидата не может быть пустым")
	}

	// Invariant: Do not replace Settings.Host with candidate technical address
	if strings.EqualFold(set.Host, cleanCand) {
		return "", errors.New("технический адрес кандидата не должен совпадать с публичным доменом")
	}

	token, err := c.stateManager.StartMigration(set.Host, cleanCand, dnsType)
	if err != nil {
		return "", err
	}

	// Prepare DNS (e.g. lower TTL) if automated provider is selected
	if dnsType == "cloudflare" && cfConfig != nil && cfConfig.APIToken != "" {
		go func() {
			adapter := NewCloudflareAdapter(*cfConfig)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := adapter.PrepareMigration(ctx, set.Host, 60); err != nil {
				slog.Warn("migration: cloudflare prepare ttl failed", "err", err)
			}
		}()
	}

	return token, nil
}

// RunVerification executes pre-switch checks against the candidate node.
func (c *Coordinator) RunVerification(ctx context.Context) ([]CheckResult, bool, error) {
	sess := c.stateManager.GetSession()
	if sess.Phase == PhaseIdle {
		return nil, false, errors.New("нет активной сессии переезда")
	}

	set, err := c.store.GetSettings()
	if err != nil {
		return nil, false, err
	}
	users, _ := c.store.ListUsers()
	nodes, _ := c.store.ListNodes()

	results, ok := c.checker.RunAllPreflightChecks(ctx, &sess, set, users, nodes)
	if err := c.stateManager.UpdateChecks(results); err != nil {
		return nil, false, err
	}

	if ok {
		_ = c.stateManager.SetPhase(PhaseCandidateReady)
	}
	return results, ok, nil
}

// ExecuteSwitchover performs the atomic cutover:
// 1. Enforces fencing on old master (halting all mutations)
// 2. Generates consistent snapshot (WAL checkpointed)
// 3. Pushes final snapshot to candidate
// 4. Promotes candidate to master
// 5. Switches DNS record
// 6. Demotes old master to Standby role
func (c *Coordinator) ExecuteSwitchover(ctx context.Context, dnsAdapter DNSAdapter, candidateSecret string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sess := c.stateManager.GetSession()
	if sess.Phase != PhaseCandidateReady {
		return fmt.Errorf("кандидат не готов к переключению (текущая фаза: %s)", sess.Phase)
	}

	// 1. Fencing: Stop ALL mutations on the old master
	slog.Info("migration: activating fencing on old master")
	if err := c.stateManager.SetFenced(true); err != nil {
		return fmt.Errorf("activate fencing: %w", err)
	}
	_ = c.stateManager.SetPhase(PhaseSwitching)

	// 2. Create consistent final snapshot
	slog.Info("migration: creating consistent snapshot")
	snapPath := filepath.Join(c.dataDir, "migration-final.tar.gz")
	defer os.Remove(snapPath)

	manifest, err := CreateConsistentSnapshot(c.dataDir, snapPath)
	if err != nil {
		_ = c.stateManager.SetError(fmt.Sprintf("snapshot error: %v", err))
		return err
	}

	// 3. Transfer snapshot to candidate
	slog.Info("migration: transferring snapshot to candidate", "candidate", sess.CandidateAddr)
	if err := c.pushSnapshotToCandidate(ctx, snapPath, sess.CandidateAddr, candidateSecret); err != nil {
		_ = c.stateManager.SetError(fmt.Sprintf("push snapshot: %v", err))
		return err
	}

	// 4. Promote candidate to active master
	slog.Info("migration: promoting candidate to active master")
	if err := c.promoteCandidate(ctx, sess.CandidateAddr, candidateSecret); err != nil {
		_ = c.stateManager.SetError(fmt.Sprintf("promote candidate: %v", err))
		return err
	}

	// 5. Switch DNS to candidate's IP
	candIP := extractIP(sess.CandidateAddr)
	if candIP != "" && dnsAdapter != nil {
		slog.Info("migration: switching DNS record", "domain", sess.PublicDomain, "new_ip", candIP)
		changeRes, err := dnsAdapter.SwitchRecord(ctx, sess.PublicDomain, candIP)
		if err != nil {
			slog.Warn("migration: automated DNS switch failed (fallback to manual)", "err", err)
		} else {
			slog.Info("migration: DNS switched successfully", "result", changeRes)
		}
	}

	// 6. Demote old master to Standby role
	slog.Info("migration: demoting local server to standby")
	_ = c.stateManager.SetRole(RoleStandby)
	_ = c.stateManager.SetPhase(PhaseStandby)
	_ = c.stateManager.UpdateStandby(func(st *StandbyStats) {
		st.SwitchedAt = time.Now().Unix()
	})

	slog.Info("migration: switchover complete, server is in standby", "users", manifest.UsersCount)
	return nil
}

func (c *Coordinator) pushSnapshotToCandidate(ctx context.Context, snapshotPath, candidateAddr, secret string) error {
	candURL := candidateAddr
	if !strings.HasPrefix(candURL, "http://") && !strings.HasPrefix(candURL, "https://") {
		candURL = "https://" + candURL
	}
	candURL = strings.TrimRight(candURL, "/") + "/migration/apply-snapshot"

	f, err := os.Open(snapshotPath)
	if err != nil {
		return err
	}
	defer f.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("snapshot", filepath.Base(snapshotPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	_ = writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, candURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if secret != "" {
		req.Header.Set("X-Migration-Secret", secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("candidate responded with HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Coordinator) promoteCandidate(ctx context.Context, candidateAddr, secret string) error {
	candURL := candidateAddr
	if !strings.HasPrefix(candURL, "http://") && !strings.HasPrefix(candURL, "https://") {
		candURL = "https://" + candURL
	}
	candURL = strings.TrimRight(candURL, "/") + "/migration/promote"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, candURL, nil)
	if err != nil {
		return err
	}
	if secret != "" {
		req.Header.Set("X-Migration-Secret", secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("candidate promotion failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// CancelRollback rolls back migration before DNS cutover.
func (c *Coordinator) CancelRollback(ctx context.Context, dnsAdapter DNSAdapter) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sess := c.stateManager.GetSession()
	if sess.Phase == PhaseCompleted {
		return errors.New("миграция уже завершена, откат невозможен")
	}

	if sess.Phase == PhaseStandby && dnsAdapter != nil && sess.OldMasterIP != "" {
		// Post-DNS rollback: revert DNS
		_ = dnsAdapter.RevertRecord(ctx, sess.PublicDomain, sess.OldMasterIP)
	}

	_ = c.stateManager.SetRole(RoleMaster)
	return c.stateManager.Rollback()
}

// extractIP extracts the IP address or host without port.
func extractIP(addr string) string {
	clean := strings.TrimSpace(addr)
	clean = strings.TrimPrefix(clean, "https://")
	clean = strings.TrimPrefix(clean, "http://")
	host, _, err := net.SplitHostPort(clean)
	if err == nil {
		return host
	}
	return clean
}
