package nodeagent

import (
	"context"
	"log/slog"

	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
	"github.com/Shu1t3/rospanel-shu1t3/internal/nodeapi"
)

// syncMTProto reconciles the embedded MTProto-proxy on the node to the desired state.
func (a *Agent) syncMTProto(m nodeapi.NodeMeta) {
	a.mtprotoMu.Lock()
	defer a.mtprotoMu.Unlock()

	if !m.MTProtoEnabled {
		if a.mtprotoOn {
			slog.Info("node: disabling embedded MTProto proxy")
			if a.mtprotoCancel != nil {
				a.mtprotoCancel()
				a.mtprotoCancel = nil
			}
			if a.mtprotoSup != nil {
				_ = a.mtprotoSup.Close()
				a.mtprotoSup = nil
			}
			a.mtprotoOn = false
			a.mtprotoPort = 0
			a.mtprotoSecret = ""
			a.mtprotoDomain = ""
			a.mtprotoMaxConns = 0
		}
		return
	}

	cfg := mtproto.Config{
		Port:     m.MTProtoPort,
		Secret:   m.MTProtoSecret,
		Domain:   m.MTProtoDomain,
		MaxConns: uint(m.MTProtoMaxConns),
	}

	// If already running with same config, nothing to do
	if a.mtprotoOn && a.mtprotoPort == m.MTProtoPort &&
		a.mtprotoSecret == m.MTProtoSecret &&
		a.mtprotoDomain == m.MTProtoDomain &&
		a.mtprotoMaxConns == m.MTProtoMaxConns {
		return
	}

	// If running, reload config atomically without parallel proxies
	if a.mtprotoOn && a.mtprotoSup != nil {
		slog.Info("node: reloading embedded MTProto proxy", "port", m.MTProtoPort)
		if err := a.mtprotoSup.Reload(cfg); err != nil {
			slog.Warn("node: reload embedded MTProto proxy failed", "err", err)
			return
		}
		a.mtprotoPort = m.MTProtoPort
		a.mtprotoSecret = m.MTProtoSecret
		a.mtprotoDomain = m.MTProtoDomain
		a.mtprotoMaxConns = m.MTProtoMaxConns
		return
	}

	// Not running: start new instance
	sup, err := mtproto.NewLifecycle(cfg)
	if err != nil {
		slog.Warn("node: failed to init embedded MTProto proxy", "err", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.mtprotoSup = sup
	a.mtprotoCancel = cancel
	a.mtprotoOn = true
	a.mtprotoPort = m.MTProtoPort
	a.mtprotoSecret = m.MTProtoSecret
	a.mtprotoDomain = m.MTProtoDomain
	a.mtprotoMaxConns = m.MTProtoMaxConns

	go func() {
		if err := sup.Run(ctx); err != nil {
			slog.Warn("node: MTProto proxy run ended", "err", err)
		}
	}()
	slog.Info("node: embedded MTProto proxy started", "port", m.MTProtoPort)
}
