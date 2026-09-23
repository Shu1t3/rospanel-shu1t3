package migration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStandbyTargetUsesCandidateHost(t *testing.T) {
	for _, tc := range []struct{ candidate, want string }{
		{"89.124.97.70:8080", "https://89.124.97.70:443"},
		{"https://candidate.example.com:8443", "https://candidate.example.com:443"},
		{"[2001:db8::1]:8080", "https://[2001:db8::1]:443"},
	} {
		got, err := StandbyTarget(tc.candidate)
		if err != nil || got != tc.want {
			t.Errorf("StandbyTarget(%q) = %q, %v; want %q", tc.candidate, got, err, tc.want)
		}
	}
}

func TestStandbyController_ServeHTTPAndStats(t *testing.T) {
	targetBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Errorf("expected X-Forwarded-For header to be set by proxy")
		}
		if r.Host != "vpn.example.com" {
			t.Errorf("proxy Host = %q, want public domain", r.Host)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/node/v1/sync" {
			t.Errorf("proxy request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-response"))
	}))
	defer targetBackend.Close()

	tmpDir := t.TempDir()
	sm, err := NewStateManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create StateManager: %v", err)
	}
	if _, err := sm.StartMigration("vpn.example.com", "127.0.0.1:8080", "manual"); err != nil {
		t.Fatal(err)
	}

	sc, err := NewStandbyController(sm, targetBackend.URL)
	if err != nil {
		t.Fatalf("failed to create StandbyController: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/node/v1/sync", strings.NewReader(`{"report":{}}`))
	rr := httptest.NewRecorder()

	sc.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "backend-response" {
		t.Fatalf("unexpected body: %s", body)
	}

	sc.RecordTraffic(500, 1500)
	sess := sm.GetSession()
	if sess.Standby.TrafficUpStandby != 500 || sess.Standby.TrafficDownStandby != 1500 {
		t.Errorf("traffic stats mismatch: %+v", sess.Standby)
	}
	if sess.Standby.LastSeenClientAt == 0 {
		t.Errorf("expected LastSeenClientAt to be populated")
	}

	// Test Decommission with active clients
	sess.Standby.ActiveClients = 3
	_ = sm.UpdateStandby(func(s *StandbyStats) { s.ActiveClients = 3 })
	if err := sc.Decommission(context.Background(), false); err == nil {
		t.Fatalf("expected error when decommissioning with active clients without force")
	}

	if err := sc.Decommission(context.Background(), true); err != nil {
		t.Fatalf("force decommission failed: %v", err)
	}

	if sm.GetSession().Role != RoleDecommissioned {
		t.Fatalf("expected role %s, got %s", RoleDecommissioned, sm.GetSession().Role)
	}
}
