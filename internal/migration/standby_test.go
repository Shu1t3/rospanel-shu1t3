package migration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStandbyController_ServeHTTPAndStats(t *testing.T) {
	targetBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Errorf("expected X-Forwarded-For header to be set by proxy")
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

	sc, err := NewStandbyController(sm, targetBackend.URL)
	if err != nil {
		t.Fatalf("failed to create StandbyController: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
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
