package core

import (
	"context"
	"fmt"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
)

// syncMasterMTProto reconciles the master's embedded MTProto proxy to the desired state.
func (m *Manager) syncMasterMTProto(cfg model.MTProtoConfig) error {
	m.mtprotoMu.Lock()
	defer m.mtprotoMu.Unlock()

	if !cfg.Enabled {
		if m.mtprotoSup != nil {
			logInfo("master: disabling embedded MTProto proxy")
			if m.mtprotoCancel != nil {
				m.mtprotoCancel()
				m.mtprotoCancel = nil
			}
			_ = m.mtprotoSup.Close()
			m.mtprotoSup = nil
		}
		return nil
	}

	protoCfg := mtproto.Config{
		Port:     cfg.Port,
		Secret:   cfg.Secret,
		Domain:   cfg.Domain,
		MaxConns: uint(cfg.MaxConns),
	}

	if m.mtprotoSup != nil {
		logInfo("master: reloading embedded MTProto proxy", "port", cfg.Port)
		if err := m.mtprotoSup.Reload(protoCfg); err != nil {
			logWarn("master: reload embedded MTProto proxy failed", "err", err)
			return err
		}
		return nil
	}

	sup, err := mtproto.NewLifecycle(protoCfg)
	if err != nil {
		logWarn("master: failed to init embedded MTProto proxy", "err", err)
		return fmt.Errorf("init master mtproto: %w", err)
	}
	m.mtprotoSup = sup

	ctx, cancel := context.WithCancel(context.Background())
	m.mtprotoCancel = cancel

	go func() {
		if err := sup.Run(ctx); err != nil {
			logWarn("master: MTProto proxy run ended", "err", err)
		}
	}()
	logInfo("master: embedded MTProto proxy started", "port", cfg.Port)
	return nil
}

// ListAllMTProtoProxies returns all MTProto proxies (standalone, master, nodes) enriched
// with dynamic runtime statistics (active connections, uptime, memory, traffic).
func (m *Manager) ListAllMTProtoProxies() ([]model.MTProtoProxy, error) {
	proxies, err := m.store.ListAllMTProtoProxies()
	if err != nil {
		return nil, err
	}

	m.mtprotoMu.Lock()
	var masterSnap *mtproto.Snapshot
	if m.mtprotoSup != nil {
		s := m.mtprotoSup.Snapshot()
		masterSnap = &s
	}
	m.mtprotoMu.Unlock()

	m.nodeGeoMu.Lock()
	nodeStats := make(map[int64]mtproto.Snapshot, len(m.nodeMTProtoStats))
	for k, v := range m.nodeMTProtoStats {
		nodeStats[k] = v
	}
	m.nodeGeoMu.Unlock()

	for i := range proxies {
		p := &proxies[i]
		if p.ID == 0 && masterSnap != nil {
			p.Running = masterSnap.Running
			p.ActiveConns = masterSnap.ActiveConns
			p.BytesRead = masterSnap.BytesRead
			p.BytesWritten = masterSnap.BytesWritten
			p.UptimeSec = masterSnap.UptimeSec
			p.MemAlloc = masterSnap.MemAlloc
			p.RSS = masterSnap.RSS
			p.LastError = masterSnap.LastError
		} else if p.ID < 0 {
			nodeID := -p.ID
			if snap, ok := nodeStats[nodeID]; ok {
				p.Running = snap.Running
				p.ActiveConns = snap.ActiveConns
				p.BytesRead = snap.BytesRead
				p.BytesWritten = snap.BytesWritten
				p.UptimeSec = snap.UptimeSec
				p.MemAlloc = snap.MemAlloc
				p.RSS = snap.RSS
				p.LastError = snap.LastError
			}
		}
	}

	return proxies, nil
}
