package migration

import (
	"testing"
	"time"
)

func TestMigrationStateLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	sm, err := NewStateManager(tmpDir)
	if err != nil {
		t.Fatalf("NewStateManager: %v", err)
	}

	sess := sm.GetSession()
	if sess.Phase != PhaseIdle || sess.Role != RoleMaster {
		t.Errorf("initial state must be idle/master, got phase=%s role=%s", sess.Phase, sess.Role)
	}

	// 1. Start migration
	pairToken, err := sm.StartMigration("vpn.example.com", "192.0.2.10:8080", "cloudflare")
	if err != nil {
		t.Fatalf("StartMigration: %v", err)
	}
	if pairToken == "" {
		t.Fatal("expected non-empty pair token")
	}

	sess = sm.GetSession()
	if sess.Phase != PhasePrepare {
		t.Errorf("expected phase prepare, got %s", sess.Phase)
	}
	if sess.PublicDomain != "vpn.example.com" {
		t.Errorf("expected domain vpn.example.com, got %s", sess.PublicDomain)
	}
	if sess.CandidateAddr != "192.0.2.10:8080" {
		t.Errorf("expected candidate 192.0.2.10:8080, got %s", sess.CandidateAddr)
	}
	// The master may restart before the final snapshot; transport pinning still
	// needs the pairing token after reloading the migration state.
	reloaded, err := NewStateManager(tmpDir)
	if err != nil || reloaded.GetSession().PairToken != pairToken {
		t.Fatalf("pair token did not survive restart: %v", err)
	}

	// Cannot start second migration while active
	if _, err := sm.StartMigration("other.com", "192.0.2.20", "manual"); err == nil {
		t.Error("expected error when starting concurrent migration, got nil")
	}

	// 2. Validate pair token (one-time use)
	if !sm.ValidatePairToken(pairToken) {
		t.Error("expected pair token to be valid")
	}
	// Second use must fail
	if sm.ValidatePairToken(pairToken) {
		t.Error("expected pair token to be consumed and rejected on second use")
	}

	// 3. Fencing test
	if sm.IsFenced() {
		t.Error("fencing should be false initially")
	}
	if err := sm.SetFenced(true); err != nil {
		t.Fatalf("SetFenced(true): %v", err)
	}
	if !sm.IsFenced() {
		t.Error("fencing should be true")
	}

	// 4. Persistence reload test
	sm2, err := NewStateManager(tmpDir)
	if err != nil {
		t.Fatalf("reload NewStateManager: %v", err)
	}
	sess2 := sm2.GetSession()
	if !sess2.Fenced || sess2.Phase != PhasePrepare {
		t.Errorf("reloaded session state mismatch: fenced=%v, phase=%s", sess2.Fenced, sess2.Phase)
	}

	// 5. Rollback
	if err := sm.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	sess = sm.GetSession()
	if sess.Phase != PhaseRolledBack || sess.Fenced {
		t.Errorf("expected rolled_back and unfenced, got phase=%s fenced=%v", sess.Phase, sess.Fenced)
	}
}

func TestStandbyStatsUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	sm, _ := NewStateManager(tmpDir)
	_, _ = sm.StartMigration("vpn.example.com", "192.0.2.10", "manual")
	_ = sm.SetRole(RoleStandby)

	now := time.Now().Unix()
	err := sm.UpdateStandby(func(st *StandbyStats) {
		st.ActiveClients = 5
		st.LastSeenClientAt = now
		st.TrafficUpStandby = 1024
		st.TrafficDownStandby = 2048
	})
	if err != nil {
		t.Fatalf("UpdateStandby: %v", err)
	}

	sess := sm.GetSession()
	if sess.Standby.ActiveClients != 5 || sess.Standby.TrafficUpStandby != 1024 || sess.Standby.TrafficDownStandby != 2048 {
		t.Errorf("standby stats mismatch: %+v", sess.Standby)
	}
}
