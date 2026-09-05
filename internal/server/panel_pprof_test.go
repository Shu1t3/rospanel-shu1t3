package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestPprofEndpointAccess(t *testing.T) {
	rt, st := rolesTestRouter(t)
	mux := rt.panelMux()

	// 1. Unauthenticated request -> should be rejected (redirected to login or 401/403)
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/debug/pprof/", nil)
	rrUnauth := httptest.NewRecorder()
	mux.ServeHTTP(rrUnauth, reqUnauth)
	if rrUnauth.Code != http.StatusUnauthorized && rrUnauth.Code != http.StatusForbidden {
		t.Fatalf("expected unauthenticated pprof request to be rejected with 401/403, got %d", rrUnauth.Code)
	}

	// 2. Operator role (insufficient permissions, owner required) -> 403
	opCookie := signIn(t, st, "op_user", model.RoleOperator, false)
	reqOp := httptest.NewRequest(http.MethodGet, "/api/debug/pprof/", nil)
	reqOp.AddCookie(opCookie)
	rrOp := httptest.NewRecorder()
	mux.ServeHTTP(rrOp, reqOp)
	if rrOp.Code != http.StatusForbidden {
		t.Fatalf("expected operator request to be forbidden (403), got %d", rrOp.Code)
	}

	// 3. Admin role (insufficient permissions, owner required) -> 403
	adminCookie := signIn(t, st, "admin_user", model.RoleAdmin, false)
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/debug/pprof/", nil)
	reqAdmin.AddCookie(adminCookie)
	rrAdmin := httptest.NewRecorder()
	mux.ServeHTTP(rrAdmin, reqAdmin)
	if rrAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected admin request to be forbidden (403), got %d", rrAdmin.Code)
	}

	// 4. Owner role -> 200 OK (HTML index)
	ownerCookie := signIn(t, st, "owner_user", model.RoleOwner, false)
	reqOwner := httptest.NewRequest(http.MethodGet, "/api/debug/pprof/", nil)
	reqOwner.AddCookie(ownerCookie)
	rrOwner := httptest.NewRecorder()
	mux.ServeHTTP(rrOwner, reqOwner)
	if rrOwner.Code != http.StatusOK {
		t.Fatalf("expected owner request to succeed (200), got %d: %s", rrOwner.Code, rrOwner.Body.String())
	}

	// 5. Querying specific profiles (goroutine and goroutineleak) as owner -> 200 OK
	for _, prof := range []string{"goroutine", "goroutineleak", "heap", "allocs"} {
		reqProf := httptest.NewRequest(http.MethodGet, "/api/debug/pprof/"+prof+"?debug=1", nil)
		reqProf.AddCookie(ownerCookie)
		rrProf := httptest.NewRecorder()
		mux.ServeHTTP(rrProf, reqProf)
		if rrProf.Code != http.StatusOK {
			t.Fatalf("expected profile %q to return 200, got %d: %s", prof, rrProf.Code, rrProf.Body.String())
		}
	}
}
