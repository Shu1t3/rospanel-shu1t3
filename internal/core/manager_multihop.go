package core

import (
	"crypto/sha256"
	"fmt"
	"slices"

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
	if hops.Reverse != nil {
		if !hops.ValidChain(in.ServerID) || set.RealityPrivateKey == "" {
			return invalidCode("err.multiHopReverse", "проверьте роли, служебные порты и IP-адреса защищённого маршрута")
		}
		for _, id := range hops.Chain(in.ServerID) {
			if servers[id].set == nil || servers[id].set.Host == "" {
				return invalidCode("err.multiHopReverse", "проверьте роли, служебные порты и IP-адреса защищённого маршрута")
			}
		}
		return nil
	}
	if !usableMultiHop(hops.Chain(in.ServerID), set, servers) {
		return invalidCode("err.multiHopPeer", "для мульти-хопа нужны подключённые серверы, после входа должен быть доступен VLESS TCP-TLS")
	}
	return nil
}

func multiHopPeer(s cascadeServer) bool {
	return s.set != nil && s.set.Host != "" && s.set.VLESSEnabled &&
		(!s.set.TLSInsecure || s.set.TLSPinSHA256 != "")
}

func usableMultiHop(chain []int64, master *model.Settings, servers map[int64]cascadeServer) bool {
	if !model.ValidMultiHopChain(chain) || servers[chain[0]].set == nil || servers[chain[0]].set.Host == "" {
		return false
	}
	if len(chain) == 1 {
		return true
	}
	return master.RealityPrivateKey != "" && !slices.ContainsFunc(chain[1:], func(id int64) bool {
		return !multiHopPeer(servers[id])
	})
}

// A staged route gets fresh service identities on every edge. A stale
// entry can only use the old chain, even when the two versions share a node.
func (m *Manager) multiHopLeg(in model.Inbound, master *model.Settings, servers map[int64]cascadeServer, chain []int64, i int) (xray.Bridge, string) {
	hops := in.Opts.MultiHop
	from, to := chain[i-1], chain[i]
	var scope, email string
	if hops.Via == nil && hops.Revision == "" {
		role := "transit"
		if i == len(chain)-1 {
			role = "exit"
		}
		scope = fmt.Sprintf("multi-hop/%d/%d/%d/%s", in.ID, from, to, role)
		email = fmt.Sprintf("bridge-multi-hop-%d-%s", in.ID, role)
	} else {
		scope = fmt.Sprintf("multi-hop/%d/%s/%d", in.ID, hops.Revision, i)
		email = fmt.Sprintf("bridge-multi-hop-%d-%s-%d", in.ID, hops.Revision, i)
	}
	uuid, _ := serviceCreds(master, scope)
	bridge := xray.Bridge{
		Entry: from, Email: email, UUID: uuid, Lane: model.BridgeVision, Sources: m.entryAddrs(servers[from]),
	}
	return bridge, bridgeLink(model.BridgeVision, model.User{UUID: uuid}, m.overIPv4(servers[to].set))
}

func (m *Manager) multiHopBridge(in model.Inbound, master *model.Settings, servers map[int64]cascadeServer, chain []int64, i int) xray.Bridge {
	bridge, _ := m.multiHopLeg(in, master, servers, chain, i)
	hop := xray.MultiHopBridge{Exit: i == len(chain)-1}
	if !hop.Exit {
		_, hop.Next = m.multiHopLeg(in, master, servers, chain, i+1)
	}
	bridge.MultiHop = &hop
	return bridge
}

func multiHopBridgeAck(bridge xray.Bridge) string {
	if len(bridge.Sources) == 0 || (!bridge.MultiHop.Exit && bridge.MultiHop.Next == "") {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(bridgesKey([]xray.Bridge{bridge}))))
}

func (m *Manager) multiHopEntryLink(in model.Inbound, master *model.Settings, servers map[int64]cascadeServer, chain []int64) string {
	if len(chain) == 1 {
		return xray.MultiHopLocalExit
	}
	_, next := m.multiHopLeg(in, master, servers, chain, 1)
	return next
}

func multiHopEntryAck(in model.Inbound, link string) string {
	if link == "" {
		return ""
	}
	key := fmt.Sprintf("entry/%d/%s/%d/%s", in.ID, in.Opts.MultiHop.Revision, in.Port, link)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}

func (m *Manager) addMultiHopRoute(opts *xray.Options, server int64, master *model.Settings, servers map[int64]cascadeServer, in model.Inbound, first int, staged bool) {
	hops := in.Opts.MultiHop
	if hops == nil || !in.Enabled || in.Protocol != model.InbVLESS || hops.Provisioning() {
		return
	}
	if hops.Reverse != nil {
		addProtectedHop(opts, m.protectedHop(in, server, master, servers, true, true), false)
		return
	}
	chain := hops.Chain(in.ServerID)
	if first >= len(chain) {
		return
	}
	index := slices.Index(chain[first:], server)
	if index < 0 || !usableMultiHop(chain, master, servers) {
		return
	}
	for i := first + index; i < len(chain); i++ {
		if chain[i] != server {
			continue
		}
		if i == 0 {
			if opts.MultiHopLinks == nil {
				opts.MultiHopLinks = make(map[int64]string)
			}
			opts.MultiHopLinks[in.ID] = m.multiHopEntryLink(in, master, servers, chain)
			continue
		}
		bridge := m.multiHopBridge(in, master, servers, chain, i)
		opts.Bridges = append(opts.Bridges, bridge)
		if staged {
			if ack := multiHopBridgeAck(bridge); ack != "" {
				opts.MultiHopAcks = append(opts.MultiHopAcks, ack)
			}
		}
	}
}
