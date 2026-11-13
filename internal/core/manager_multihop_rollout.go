package core

import (
	"slices"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

type multiHopApplied struct {
	acks []string
	at   time.Time
}

func (m *Manager) noteMultiHopApplied(id int64, acks []string) {
	m.multiHopAppliedMu.Lock()
	defer m.multiHopAppliedMu.Unlock()
	if len(acks) == 0 {
		delete(m.multiHopApplied, id)
		return
	}
	if m.multiHopApplied == nil {
		m.multiHopApplied = make(map[int64]multiHopApplied)
	}
	m.multiHopApplied[id] = multiHopApplied{acks: slices.Clone(acks), at: time.Now()}
}

func (m *Manager) hasMultiHopApplied(id int64, ack string) bool {
	if ack == "" {
		return false
	}
	m.multiHopAppliedMu.Lock()
	seen := m.multiHopApplied[id]
	m.multiHopAppliedMu.Unlock()
	if id == model.LocalNodeID {
		return m.sup != nil && m.sup.Serving() && slices.Contains(seen.acks, ack)
	}
	return time.Since(seen.at) < time.Duration(model.NodeOnlineWindow)*time.Second && slices.Contains(seen.acks, ack)
}

func stagedMultiHopInbound(in model.Inbound, pending *model.MultiHopPending) model.Inbound {
	nodes := pending.Chain()
	hops := multiHopFromChain(nodes)
	if pending.Reverse != nil {
		hops = protectedMultiHop(nodes, pending.Reverse)
	} else if pending.Legacy {
		hops.Via = nil
	}
	hops.Revision = pending.Revision
	hops.IPFamilies = slices.Clone(pending.IPFamilies)
	in.ServerID, in.Name, in.Opts.MultiHop = nodes[0], pending.Name, &hops
	return in
}

// The last confirmed config can go stale while a rollout is in progress, for
// example if DNS drops the AAAA for a staged downstream leg.
func (m *Manager) multiHopCanSwitchEntry(in model.Inbound, master *model.Settings, servers map[int64]cascadeServer, chain []int64) bool {
	if m.multiHopEntryLink(in, master, servers, chain) == "" {
		return false
	}
	for i := 2; i < len(chain); i++ {
		if hopIPFamily(in.Opts.MultiHop.IPFamilies, i-1) != model.HopIPAuto {
			if _, link := m.multiHopLeg(in, master, servers, chain, i); link == "" {
				return false
			}
		}
	}
	return true
}

func (m *Manager) multiHopLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if err := m.advanceMultiHop(); err != nil {
				logErr("multi-hop: advancing routes", "err", err)
			}
		}
	}
}

// Deploy one more leg from the exit towards the entry. Store each step so a
// panel restart keeps already provisioned service listeners in desired state.
func (m *Manager) advanceMultiHop() error {
	m.multiHopWrites.Lock()
	defer m.multiHopWrites.Unlock()
	routes, err := m.store.DeployingMultiHopInbounds()
	if err != nil || len(routes) == 0 {
		return err
	}
	master, err := m.store.GetSettings()
	if err != nil {
		return err
	}
	nodes, err := m.store.ListNodes()
	if err != nil {
		return err
	}
	servers := m.cascadeServers(master, nodes)
	for _, in := range routes {
		if err := m.advanceMultiHopInbound(in, master, servers); err != nil {
			logErr("multi-hop: applying route", "route", in.ID, "err", err)
		}
	}
	return nil
}

func (m *Manager) advanceMultiHopInbound(in model.Inbound, master *model.Settings, servers map[int64]cascadeServer) error {
	pending := in.Opts.MultiHop.Pending
	chain := pending.Chain()
	if pending.Reverse != nil {
		if !pending.Reverse.Valid(chain) {
			return nil
		}
		return m.advanceProtectedHop(in, stagedMultiHopInbound(in, pending), master, servers)
	}
	if !usableMultiHop(chain, master, servers) {
		return nil
	}
	staged := stagedMultiHopInbound(in, pending)
	// Confirm the whole deployed suffix again before promotion, including after
	// a panel restart or a change in cert pin, addresses or bridge credentials.
	for i := len(chain) - 1; i >= max(1, len(chain)-1-pending.Ready); i-- {
		bridge := m.multiHopBridge(staged, master, servers, chain, i)
		if !m.hasMultiHopApplied(chain[i], multiHopBridgeAck(bridge)) {
			return nil
		}
	}
	if pending.Ready < len(chain)-2 {
		pending.Ready++
		if err := m.store.UpdateInbound(in); err != nil {
			return err
		}
		m.wakeMultiHopNode(chain[len(chain)-1-pending.Ready])
		return nil
	}
	link := m.multiHopEntryLink(staged, master, servers, chain)
	if link == "" {
		return nil
	}
	if !pending.ApplyEntry {
		pending.ApplyEntry = true
		if err := m.store.UpdateInbound(in); err != nil {
			return err
		}
		m.wakeMultiHopNode(staged.ServerID)
		return nil
	}
	if !m.hasMultiHopApplied(staged.ServerID, multiHopEntryAck(staged, link)) {
		return nil
	}
	return m.promoteMultiHop(in, staged)
}

func (m *Manager) promoteMultiHop(in, staged model.Inbound) error {
	// The new entry now serves the new chain. Only now publish the move
	// and remove old service legs. If the new entry cannot apply the config,
	// clients keep the old subscription and its old chain stays configured.
	if err := m.store.UpdateMultiHopInbound(staged); err != nil {
		return err
	}
	if in.ServerID == model.LocalNodeID || staged.ServerID == model.LocalNodeID {
		m.applyInboundChange(model.LocalNodeID, true)
	} else {
		m.cascadeMoved()
	}
	return nil
}

func (m *Manager) wakeMultiHopNode(id int64) {
	if id == model.LocalNodeID {
		m.ensureLocalConnGuard()
		m.TriggerReconcile()
	} else if m.nodes != nil {
		m.nodes.wakeOne(id)
	}
}
