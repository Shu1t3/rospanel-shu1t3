package core

import (
	"fmt"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func (m *Manager) validateMultiHop(in model.Inbound) error {
	hops := in.Opts.MultiHop
	if hops == nil || !in.Enabled {
		return nil
	}
	set, err := m.store.GetSettings()
	if err != nil {
		return err
	}
	nodes, err := m.store.ListNodes()
	if err != nil {
		return err
	}
	servers := m.cascadeServers(set, nodes)
	if set.RealityPrivateKey == "" || servers[in.ServerID].set == nil ||
		!multiHopPeer(servers[hops.Transit]) || !multiHopPeer(servers[hops.Exit]) {
		return invalidCode("err.multiHopPeer", "для мульти-хопа нужны подключённые серверы, на транзите и выходе должен быть доступен VLESS TCP-TLS")
	}
	return nil
}

func multiHopPeer(s cascadeServer) bool {
	return s.set != nil && s.set.Host != "" && s.set.VLESSEnabled &&
		(!s.set.TLSInsecure || s.set.TLSPinSHA256 != "")
}

func (m *Manager) multiHopOpts(opts *xray.Options, server int64, master *model.Settings, servers map[int64]cascadeServer, in model.Inbound) {
	hops := in.Opts.MultiHop
	if hops == nil || !in.Enabled || in.Protocol != model.InbVLESS || master.RealityPrivateKey == "" {
		return
	}
	entry, transit, exit := in.ServerID, hops.Transit, hops.Exit
	if entry == transit || entry == exit || transit == exit || servers[entry].set == nil ||
		!multiHopPeer(servers[transit]) || !multiHopPeer(servers[exit]) {
		return
	}
	leg := func(from, to int64, role string) (xray.Bridge, string) {
		scope := fmt.Sprintf("multi-hop/%d/%d/%d/%s", in.ID, from, to, role)
		uuid, _ := serviceCreds(master, scope)
		bridge := xray.Bridge{
			Entry: from, Email: fmt.Sprintf("bridge-multi-hop-%d-%s", in.ID, role),
			UUID: uuid, Lane: model.BridgeVision, Sources: m.entryAddrs(servers[from]),
		}
		return bridge, bridgeLink(model.BridgeVision, model.User{UUID: uuid}, m.overIPv4(servers[to].set))
	}
	switch server {
	case entry:
		_, next := leg(entry, transit, "transit")
		if opts.MultiHopLinks == nil {
			opts.MultiHopLinks = make(map[int64]string)
		}
		opts.MultiHopLinks[in.ID] = next
	case transit:
		bridge, _ := leg(entry, transit, "transit")
		_, next := leg(transit, exit, "exit")
		bridge.MultiHop = &xray.MultiHopBridge{Next: next}
		opts.Bridges = append(opts.Bridges, bridge)
	case exit:
		bridge, _ := leg(transit, exit, "exit")
		bridge.MultiHop = &xray.MultiHopBridge{Exit: true}
		opts.Bridges = append(opts.Bridges, bridge)
	}
}
