package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
)

func TestMTProtoAPIEndpoints(t *testing.T) {
	rt, st := rolesTestRouter(t)
	h := rt.panelMux()
	admin := signIn(t, st, "admin", model.RoleAdmin, false)

	// 1. Generate FakeTLS Secret
	genReq := httptest.NewRequest("POST", "/api/mtproto/gen-secret?domain=cloudflare.com", nil)
	genReq.AddCookie(admin)
	genRec := httptest.NewRecorder()
	h.ServeHTTP(genRec, genReq)
	if genRec.Code != http.StatusOK {
		t.Fatalf("generate secret failed: %d (%s)", genRec.Code, genRec.Body.String())
	}
	var genResp map[string]string
	if err := json.Unmarshal(genRec.Body.Bytes(), &genResp); err != nil {
		t.Fatalf("unmarshal genResp: %v", err)
	}
	sec := genResp["secret"]
	if len(sec) < 32 {
		t.Fatalf("unexpected generated secret: %s", sec)
	}

	// 2. Create standalone proxy
	createBody := `{"name":"Proxy 1","host":"198.51.100.1","port":8443,"secret":"` + sec + `","domain":"cloudflare.com","max_conns":512,"enabled":true}`
	createReq := httptest.NewRequest("POST", "/api/mtproto", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(admin)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create proxy failed: %d (%s)", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		Proxy          mtprotoProxyItem `json:"proxy"`
		InstallCommand string           `json:"install_command"`
		CLICommand     string           `json:"cli_command"`
		Token          string           `json:"token"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal createResp: %v", err)
	}
	if createResp.Proxy.ID <= 0 || createResp.Token == "" {
		t.Fatalf("unexpected createResp: %+v", createResp)
	}
	proxyID := createResp.Proxy.ID
	proxyToken := createResp.Token

	// 3. List proxies
	listReq := httptest.NewRequest("GET", "/api/mtproto", nil)
	listReq.AddCookie(admin)
	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list proxies failed: %d", listRec.Code)
	}
	var listResp struct {
		Proxies []mtprotoProxyItem `json:"proxies"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal listResp: %v", err)
	}
	if len(listResp.Proxies) != 1 || listResp.Proxies[0].ID != proxyID {
		t.Fatalf("unexpected listResp: %+v", listResp)
	}

	// 4. Get single proxy
	getReq := httptest.NewRequest("GET", "/api/mtproto/"+itoa64(proxyID), nil)
	getReq.AddCookie(admin)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get proxy failed: %d", getRec.Code)
	}

	// 5. Update proxy
	updateBody := `{"name":"Proxy 1 Renamed","host":"198.51.100.2","port":9443,"secret":"` + sec + `","domain":"cloudflare.com","max_conns":1024,"enabled":true}`
	updateReq := httptest.NewRequest("PUT", "/api/mtproto/"+itoa64(proxyID), strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(admin)
	updateRec := httptest.NewRecorder()
	h.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update proxy failed: %d (%s)", updateRec.Code, updateRec.Body.String())
	}

	// 6. Standalone runner sync (POST /api/mtproto/sync)
	// 6a. Unauthorized without bearer token
	syncNoAuth := httptest.NewRequest("POST", "/api/mtproto/sync", strings.NewReader(`{}`))
	syncNoAuthRec := httptest.NewRecorder()
	h.ServeHTTP(syncNoAuthRec, syncNoAuth)
	if syncNoAuthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", syncNoAuthRec.Code)
	}

	// 6b. Authorized heartbeat with token
	hbBody := mtproto.HeartbeatRequest{
		Token:      proxyToken,
		ConfigHash: "outdated-hash",
		Snapshot: mtproto.Snapshot{
			Running:      true,
			ActiveConns:  15,
			BytesRead:    5000,
			BytesWritten: 10000,
			UptimeSec:    60,
			MemAlloc:     12 * 1024 * 1024,
			RSS:          22 * 1024 * 1024,
		},
	}
	hbJSON, _ := json.Marshal(hbBody)
	syncReq := httptest.NewRequest("POST", "/api/mtproto/sync", strings.NewReader(string(hbJSON)))
	syncReq.Header.Set("Content-Type", "application/json")
	syncReq.Header.Set("Authorization", "Bearer "+proxyToken)
	syncRec := httptest.NewRecorder()
	h.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync failed: %d (%s)", syncRec.Code, syncRec.Body.String())
	}
	var syncResp mtproto.HeartbeatResponse
	if err := json.Unmarshal(syncRec.Body.Bytes(), &syncResp); err != nil {
		t.Fatalf("unmarshal syncResp: %v", err)
	}
	if !syncResp.OK || !syncResp.ConfigChanged || syncResp.NewConfig == nil || syncResp.NewConfig.Port != 9443 {
		t.Fatalf("unexpected syncResp: %+v", syncResp)
	}

	// 7. Toggle proxy enabled
	toggleReq := httptest.NewRequest("POST", "/api/mtproto/"+itoa64(proxyID)+"/toggle", strings.NewReader(`{"enabled":false}`))
	toggleReq.Header.Set("Content-Type", "application/json")
	toggleReq.AddCookie(admin)
	toggleRec := httptest.NewRecorder()
	h.ServeHTTP(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusOK {
		t.Fatalf("toggle failed: %d", toggleRec.Code)
	}

	// 8. Delete proxy
	delReq := httptest.NewRequest("DELETE", "/api/mtproto/"+itoa64(proxyID), nil)
	delReq.AddCookie(admin)
	delRec := httptest.NewRecorder()
	h.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete failed: %d", delRec.Code)
	}
}
