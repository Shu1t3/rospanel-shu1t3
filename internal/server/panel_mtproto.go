package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
)

type mtprotoProxyItem struct {
	model.MTProtoProxy
	Link      string `json:"link"`
	HTTPSLink string `json:"https_link"`
}

func toMTProtoProxyItem(p model.MTProtoProxy) mtprotoProxyItem {
	return mtprotoProxyItem{
		MTProtoProxy: p,
		Link:         p.Link(),
		HTTPSLink:    p.HTTPSLink(),
	}
}

// listMTProto returns all MTProto proxies: standalone proxies plus mixed-mode nodes.
func (rt *Router) listMTProto(w http.ResponseWriter, _ *http.Request) {
	proxies, err := rt.mgr.ListAllMTProtoProxies()
	if err != nil {
		writeManagerErr(w, err)
		return
	}
	items := make([]mtprotoProxyItem, len(proxies))
	for i, p := range proxies {
		items[i] = toMTProtoProxyItem(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxies": items})
}

type mtprotoCreateReq struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Secret   string `json:"secret"`
	Domain   string `json:"domain"`
	MaxConns int    `json:"max_conns"`
	Enabled  bool   `json:"enabled"`
}

// createMTProto persists a new standalone MTProto proxy and generates an install command.
func (rt *Router) createMTProto(w http.ResponseWriter, r *http.Request) {
	var req mtprotoCreateReq
	if !decodeJSON(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		req.Domain = model.DefaultFakeTLSDomain
	}
	if req.Port <= 0 {
		req.Port = 8443
	}
	if req.MaxConns <= 0 {
		req.MaxConns = 512
	}
	if req.Secret == "" {
		sec, err := mtproto.GenerateSecret(req.Domain)
		if err != nil {
			writeErrCode(w, http.StatusBadRequest, "err.badSecret", "не удалось сгенерировать FakeTLS секрет")
			return
		}
		req.Secret = sec
	}

	p := model.MTProtoProxy{
		Name:     req.Name,
		Host:     req.Host,
		Port:     req.Port,
		Secret:   req.Secret,
		Domain:   req.Domain,
		MaxConns: req.MaxConns,
		Enabled:  req.Enabled,
	}

	if err := rt.mgr.Store().CreateMTProtoProxy(&p); err != nil {
		writeManagerErr(w, err)
		return
	}

	syncURL := panelPublicURL(r) + "/api/mtproto/sync"
	installCmd := "curl -Ls https://raw.githubusercontent.com/" + updateRepo() +
		"/main/install.sh | sudo bash -s -- mtproto run --sync-url '" + syncURL +
		"' --sync-token '" + p.Token + "'"
	cliCmd := "./rospanel mtproto run --sync-url '" + syncURL + "' --sync-token '" + p.Token + "'"

	writeJSON(w, http.StatusCreated, map[string]any{
		"proxy":           toMTProtoProxyItem(p),
		"install_command": installCmd,
		"cli_command":     cliCmd,
		"token":           p.Token,
	})
}

// getMTProto returns one standalone proxy by ID.
func (rt *Router) getMTProto(w http.ResponseWriter, _ *http.Request, id int64) {
	p, err := rt.mgr.Store().GetMTProtoProxy(id)
	if err != nil {
		writeManagerErr(w, err)
		return
	}
	if p == nil {
		writeErrCode(w, http.StatusNotFound, "err.proxyNotFound", "прокси не найден")
		return
	}
	writeJSON(w, http.StatusOK, toMTProtoProxyItem(*p))
}

// updateMTProto edits a standalone proxy's settings.
func (rt *Router) updateMTProto(w http.ResponseWriter, r *http.Request, id int64) {
	var req mtprotoCreateReq
	if !decodeJSON(w, r, &req) {
		return
	}

	p, err := rt.mgr.Store().GetMTProtoProxy(id)
	if err != nil {
		writeManagerErr(w, err)
		return
	}
	if p == nil {
		writeErrCode(w, http.StatusNotFound, "err.proxyNotFound", "прокси не найден")
		return
	}

	p.Name = strings.TrimSpace(req.Name)
	p.Host = strings.TrimSpace(req.Host)
	if req.Port > 0 {
		p.Port = req.Port
	}
	if req.Secret != "" {
		p.Secret = strings.TrimSpace(req.Secret)
	}
	if req.Domain != "" {
		p.Domain = strings.TrimSpace(req.Domain)
	}
	if req.MaxConns > 0 {
		p.MaxConns = req.MaxConns
	}
	p.Enabled = req.Enabled

	if err := rt.mgr.Store().UpdateMTProtoProxy(*p); err != nil {
		writeManagerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMTProtoProxyItem(*p))
}

// deleteMTProto removes a standalone proxy.
func (rt *Router) deleteMTProto(w http.ResponseWriter, _ *http.Request, id int64) {
	if err := rt.mgr.Store().DeleteMTProtoProxy(id); err != nil {
		writeManagerErr(w, err)
		return
	}
	writeOK(w)
}

// toggleMTProto toggles the enabled switch of a proxy (standalone or mixed node).
func (rt *Router) toggleMTProto(w http.ResponseWriter, r *http.Request, id int64) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if id == 0 {
		set, err := rt.mgr.Store().GetSettings()
		if err != nil {
			writeManagerErr(w, err)
			return
		}
		cfg := set.MTProto
		cfg.Enabled = req.Enabled
		if err := rt.mgr.SetMTProtoProxy(0, cfg); err != nil {
			writeManagerErr(w, err)
			return
		}
		writeOK(w)
		return
	}

	// Negative ID indicates a node proxy in mixed mode
	if id < 0 {
		nodeID := -id
		node, err := rt.mgr.GetNode(nodeID)
		if err != nil || node == nil {
			writeErrCode(w, http.StatusNotFound, "err.nodeNotFound", "нода не найдена")
			return
		}
		cfg := node.MTProto
		cfg.Enabled = req.Enabled
		if err := rt.mgr.SetMTProtoProxy(nodeID, cfg); err != nil {
			writeManagerErr(w, err)
			return
		}
		writeOK(w)
		return
	}

	if err := rt.mgr.Store().SetMTProtoProxyEnabled(id, req.Enabled); err != nil {
		writeManagerErr(w, err)
		return
	}
	writeOK(w)
}

// generateMTProtoSecret creates a fresh FakeTLS secret.
func (rt *Router) generateMTProtoSecret(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		var body struct {
			Domain string `json:"domain"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		domain = body.Domain
	}
	if strings.TrimSpace(domain) == "" {
		domain = model.DefaultFakeTLSDomain
	}

	sec, err := mtproto.GenerateSecret(domain)
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, "err.generateSecret", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": sec,
		"domain": domain,
	})
}

// setServerMTProto updates MTProto settings for a node (or master if id == 0).
func (rt *Router) setServerMTProto(w http.ResponseWriter, r *http.Request, id int64) {
	var cfg model.MTProtoConfig
	if !decodeJSON(w, r, &cfg) {
		return
	}
	if err := rt.mgr.SetMTProtoProxy(id, cfg); err != nil {
		writeManagerErr(w, err)
		return
	}
	writeOK(w)
}

// syncMTProtoStandalone receives heartbeats and returns config updates for standalone proxies.
func (rt *Router) syncMTProtoStandalone(w http.ResponseWriter, r *http.Request) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	token := authHeader
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		token = strings.TrimSpace(authHeader[7:])
	}
	if token == "" {
		writeErrCode(w, http.StatusUnauthorized, "err.authRequired", "токен авторизации отсутствует")
		return
	}

	var req mtproto.HeartbeatRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	p, err := rt.mgr.Store().GetMTProtoProxyByToken(token)
	if err != nil || p == nil {
		writeErrCode(w, http.StatusUnauthorized, "err.invalidToken", "неверный токен прокси")
		return
	}

	// Update statistics in DB
	_ = rt.mgr.Store().UpdateMTProtoHeartbeat(token, req.Snapshot)

	desiredCfg := mtproto.Config{
		Port:      p.Port,
		Secret:    p.Secret,
		Domain:    p.Domain,
		MaxConns:  uint(p.MaxConns),
		SyncToken: token,
	}

	changed := desiredCfg.ConfigHash() != req.ConfigHash
	var newCfg *mtproto.Config
	if changed {
		newCfg = &desiredCfg
	}

	resp := mtproto.HeartbeatResponse{
		OK:            true,
		ConfigChanged: changed,
		NewConfig:     newCfg,
	}
	writeJSON(w, http.StatusOK, resp)
}
