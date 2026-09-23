package migration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
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
	publicHost   string
	proxy        *httputil.ReverseProxy
	lastSeen     atomic.Int64
}

// StandbyTarget routes through the candidate's IP after promotion. Looking up
// the public domain on the old master can resolve back to the old IP while DNS
// caches expire, causing the standby proxy to call itself.
func StandbyTarget(candidateAddr string) (string, error) {
	candidateURL, err := CandidateURL(candidateAddr)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(candidateURL)
	if err != nil {
		return "", err
	}
	return "https://" + net.JoinHostPort(u.Hostname(), "443"), nil
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
		publicHost:   sm.GetSession().PublicDomain,
	}

	sc.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			if sc.publicHost != "" {
				pr.Out.Host = sc.publicHost
			}
			pr.SetXForwarded()
			sc.RecordRequest(pr.In)
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         sc.publicHost,
				InsecureSkipVerify: true, // the transport dials the candidate's technical IP
			},
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
