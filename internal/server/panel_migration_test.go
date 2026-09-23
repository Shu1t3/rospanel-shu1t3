package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestMigrationEndpointsAndFencing(t *testing.T) {
	rt, st := rolesTestRouter(t)
	// Initialize migration coordinator
	if err := rt.InitMigration(rt.dataDir); err != nil {
		t.Fatalf("InitMigration: %v", err)
	}

	h := rt.panelMux()
	ownerCookie := signIn(t, st, "owner", model.RoleOwner, false)

	// 1. Initial status: idle
	req := httptest.NewRequest(http.MethodGet, "/api/migration/status", nil)
	req.AddCookie(ownerCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var sess modelStateSession
	if err := json.NewDecoder(rec.Body).Decode(&sess); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if sess.Phase != "idle" {
		t.Errorf("initial phase = %s, want idle", sess.Phase)
	}

	// 2. Start migration
	startPayload := `{"candidate_addr": "192.0.2.55:8080", "dns_type": "manual"}`
	sreq := httptest.NewRequest(http.MethodPost, "/api/migration/start", strings.NewReader(startPayload))
	sreq.Header.Set("Content-Type", "application/json")
	sreq.AddCookie(ownerCookie)
	srec := httptest.NewRecorder()
	h.ServeHTTP(srec, sreq)
	if srec.Code != http.StatusOK {
		t.Fatalf("start code = %d, want 200 (body: %s)", srec.Code, srec.Body.String())
	}
	var sresp map[string]string
	_ = json.NewDecoder(srec.Body).Decode(&sresp)
	if sresp["pair_token"] == "" || !strings.Contains(sresp["install_cmd"], "--candidate") {
		t.Errorf("unexpected start response: %+v", sresp)
	}
	statusReq := httptest.NewRequest(http.MethodGet, "/api/migration/status", nil)
	statusReq.AddCookie(ownerCookie)
	statusRec := httptest.NewRecorder()
	h.ServeHTTP(statusRec, statusReq)
	if strings.Contains(statusRec.Body.String(), sresp["pair_token"]) {
		t.Fatal("migration status disclosed the pairing token")
	}

	// 3. Fencing check: when fenced, non-migration POST is blocked with 503
	rt.mgr.SetFenced(true)
	createReq := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"name":"fenced_user"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(ownerCookie)
	crec := httptest.NewRecorder()
	rt.ServeHTTP(crec, createReq)
	if crec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable when fenced, got %d", crec.Code)
	}
	if !strings.Contains(crec.Body.String(), "server_fenced") {
		t.Errorf("expected server_fenced error message, got %s", crec.Body.String())
	}

	// But migration endpoint itself is allowed when fenced
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/migration/rollback", nil)
	rollbackReq.AddCookie(ownerCookie)
	rrec := httptest.NewRecorder()
	h.ServeHTTP(rrec, rollbackReq)
	if rrec.Code != http.StatusOK {
		t.Fatalf("rollback code = %d, want 200 (body: %s)", rrec.Code, rrec.Body.String())
	}

	// Unfenced again
	if rt.mgr.IsFenced() {
		t.Error("fencing should be cleared after rollback")
	}

}

type modelStateSession struct {
	Phase string `json:"phase"`
	Role  string `json:"role"`
}
