package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/core"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestMultiHopPanelOwnsExistingRoutes(t *testing.T) {
	t.Parallel()
	rt, st := rolesTestRouter(t)
	h := rt.panelMux()
	admin := signIn(t, st, "admin", model.RoleAdmin, false)
	old := model.Inbound{
		Name: "Legacy", Port: 18443, Protocol: model.InbVLESS,
		Opts: model.InboundOpts{
			Transport: model.TrWS, Security: model.SecTLS, Path: "/legacy",
			MultiHop: &model.MultiHop{Transit: 1, Exit: 2},
		},
	}
	old.Normalize()
	saved, err := st.CreateInbound(old)
	if err != nil {
		t.Fatal(err)
	}
	grant := model.InboundToken(saved.ID)
	group, err := st.CreateGroup("Pilot", []string{grant}, 0)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, status int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(admin)
		req.Header.Set("X-RosPanel-CSRF", "1")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, rec.Code, status, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	var routes []core.MultiHopProfile
	body := request(http.MethodGet, "/api/multi-hop", "", http.StatusOK)
	if err := json.Unmarshal(body, &routes); err != nil || len(routes) != 1 || routes[0].ID != saved.ID {
		t.Fatalf("legacy route missing: %s (%v)", body, err)
	}
	body = request(http.MethodGet, "/api/servers/0/inbounds", "", http.StatusOK)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("the ordinary editor received the route: %s", body)
	}
	request(http.MethodDelete, fmt.Sprintf("/api/inbounds/%d", saved.ID), "", http.StatusBadRequest)
	request(http.MethodPost, "/api/multi-hop", `{"entry":1,"transit":2,"name":"missing exit"}`, http.StatusBadRequest)
	request(http.MethodPost, fmt.Sprintf("/api/multi-hop/%d", saved.ID),
		`{"name":"Renamed","transit":1,"exit":3,"enabled":false}`, http.StatusOK)
	updated, err := st.GetInbound(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Opts.MultiHop == nil || updated.Opts.MultiHop.Exit != 3 ||
		updated.Port != old.Port || updated.Opts.Transport != old.Opts.Transport || updated.Opts.Path != old.Opts.Path {
		t.Fatalf("the entry changed on route edit: %+v", updated)
	}
	editPath := fmt.Sprintf("/api/multi-hop/%d", saved.ID)
	body = request(http.MethodPost, editPath, `{"name":"Return","nodes":[0,1,0],"enabled":false}`, http.StatusOK)
	var route core.MultiHopProfile
	if err := json.Unmarshal(body, &route); err != nil || !slices.Equal(route.Nodes, []int64{0, 1, 0}) {
		t.Fatalf("return route missing: %s (%v)", body, err)
	}
	for _, chain := range []string{`[0,0]`, `[0,1,0,2]`, `[0,1,1,0]`} {
		request(http.MethodPost, editPath, `{"name":"Invalid","nodes":`+chain+`,"enabled":false}`, http.StatusBadRequest)
	}
	group, err = st.GetGroup(group.ID)
	if err != nil || group == nil || len(group.Grants) != 1 || group.Grants[0] != grant {
		t.Fatalf("group grant lost: %+v (%v)", group, err)
	}
}
