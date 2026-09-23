package migration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"
)

// StandbyController manages the standby lifecycle on the old master server.
type StandbyController struct {
	stateManager *StateManager
	newMasterURL *url.URL
	proxy        *httputil.ReverseProxy
	lastSeen     atomic.Int64
}

// NewStandbyController builds a controller that reverse-proxies control requests
// to the new master while keeping local VPN services alive for cached clients.
func NewStandbyController(sm *StateManager, newMasterTarget string) (*StandbyController, error) {
	target, err := url.Parse(newMasterTarget)
	if err != nil {
		return nil, fmt.Errorf("invalid new master URL: %w", err)
	}

	sc := &StandbyController{
		stateManager: sm,
		newMasterURL: target,
	}

	sc.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			sc.RecordRequest(pr.In)
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // may use technical IP during migration window
		},
	}

	return sc, nil
}

// RecordRequest tracks activity from clients still hitting the old server IP.
func (sc *StandbyController) RecordRequest(req *http.Request) {
	now := time.Now().Unix()
	sc.lastSeen.Store(now)

	_ = sc.stateManager.UpdateStandby(func(s *StandbyStats) {
		s.LastSeenClientAt = now
	})
}

// RecordTraffic updates traffic bytes carried by the standby server.
func (sc *StandbyController) RecordTraffic(up, down int64) {
	_ = sc.stateManager.UpdateStandby(func(s *StandbyStats) {
		s.TrafficUpStandby += up
		s.TrafficDownStandby += down
		s.LastSyncAt = time.Now().Unix()
	})
}

// ServeHTTP delegates control requests to the new master.
func (sc *StandbyController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sc.proxy.ServeHTTP(w, r)
}

// Decommission permanently stops standby mode and invalidates migration artifacts.
func (sc *StandbyController) Decommission(ctx context.Context, force bool) error {
	sess := sc.stateManager.GetSession()
	if !force && sess.Standby.ActiveClients > 0 {
		return fmt.Errorf("старый сервер все еще обслуживает %d активных клиентов (используйте force)", sess.Standby.ActiveClients)
	}

	if err := sc.stateManager.SetRole(RoleDecommissioned); err != nil {
		return err
	}
	return sc.stateManager.Complete()
}
