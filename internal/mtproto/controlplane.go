package mtproto

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// HeartbeatRequest is sent by the standalone proxy to the control-plane panel.
type HeartbeatRequest struct {
	Token      string   `json:"token"`
	ConfigHash string   `json:"config_hash"`
	Snapshot   Snapshot `json:"snapshot"`
}

// HeartbeatResponse is returned by the control-plane panel to the proxy.
type HeartbeatResponse struct {
	OK            bool    `json:"ok"`
	ConfigChanged bool    `json:"config_changed"`
	NewConfig     *Config `json:"new_config,omitempty"`
	Message       string  `json:"message,omitempty"`
}

// ControlPlane manages background synchronization with a remote RosPanel controller.
type ControlPlane struct {
	lifecycle  *Lifecycle
	httpClient *http.Client
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewControlPlane initializes the control plane client.
func NewControlPlane(l *Lifecycle) *ControlPlane {
	return &ControlPlane{
		lifecycle: l,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// Start begins periodic heartbeat synchronization if RemoteSyncURL is configured.
func (cp *ControlPlane) Start(ctx context.Context) {
	cfg := cp.lifecycle.CurrentConfig()
	if cfg.RemoteSyncURL == "" {
		slog.Debug("mtproto control-plane: remote sync url not configured, running standalone")
		return
	}

	interval := cfg.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}

	cp.wg.Add(1)
	go func() {
		defer cp.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial check-in immediately
		cp.syncOnce(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-cp.stopCh:
				return
			case <-ticker.C:
				cp.syncOnce(ctx)
			}
		}
	}()
}

// Stop gracefully shuts down the control plane loop.
func (cp *ControlPlane) Stop() {
	close(cp.stopCh)
	cp.wg.Wait()
}

func (cp *ControlPlane) syncOnce(ctx context.Context) {
	cfg := cp.lifecycle.CurrentConfig()
	if cfg.RemoteSyncURL == "" {
		return
	}

	reqBody := HeartbeatRequest{
		Token:      cfg.SyncToken,
		ConfigHash: cfg.ConfigHash(),
		Snapshot:   cp.lifecycle.Snapshot(),
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		slog.Warn("mtproto control-plane: marshal heartbeat error", "err", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.RemoteSyncURL, bytes.NewReader(data))
	if err != nil {
		slog.Warn("mtproto control-plane: create request error", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.SyncToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.SyncToken)
	}

	resp, err := cp.httpClient.Do(req)
	if err != nil {
		slog.Warn("mtproto control-plane: sync request failed", "url", cfg.RemoteSyncURL, "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("mtproto control-plane: unexpected response status", "status", resp.StatusCode)
		return
	}

	var syncResp HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		slog.Warn("mtproto control-plane: decode response error", "err", err)
		return
	}

	if syncResp.ConfigChanged && syncResp.NewConfig != nil {
		newCfg := *syncResp.NewConfig
		if newCfg.ConfigHash() != cfg.ConfigHash() {
			slog.Info("mtproto control-plane: remote config changed, reloading proxy")
			if rErr := cp.lifecycle.Reload(newCfg); rErr != nil {
				slog.Error("mtproto control-plane: reload failed", "err", rErr)
			}
		}
	}
}
