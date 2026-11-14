package core

import (
	"crypto/sha256"
	"fmt"
	"net/netip"
	"slices"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func (m *Manager) validateMultiHop(in model.Inbound) error {
	hops := in.Opts.MultiHop
	if hops == nil {
		return nil
	}
	chain := hops.Chain(in.ServerID)
	protected := hops.Reverse != nil
	if !in.Enabled && len(hops.IPFamilies) == 0 {
		return nil
	}
	if protected && !hops.Reverse.Valid(chain) {
		return invalidCode("err.multiHopReverse", "проверьте роли, служебные порты и IP-адреса защищённого маршрута")
	}
	if _, err := normalizedHopFamilies(hops.IPFamilies, len(model.MultiHopEdges(chain, protected))); err != nil || !in.Enabled {
		return err
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
	if protected {
		if set.RealityPrivateKey == "" || slices.ContainsFunc(chain, func(id int64) bool {
			return servers[id].set == nil || servers[id].set.Host == ""
		}) {
			return invalidCode("err.multiHopReverse", "проверьте роли, служебные порты и IP-адреса защищённого маршрута")
		}
	} else if !usableMultiHop(chain, set, servers) {
		return invalidCode("err.multiHopPeer", "для мульти-хопа нужны подключённые серверы, после входа должен быть доступен VLESS TCP-TLS")
	}
	for i, edge := range model.MultiHopEdges(chain, protected) {
		family := hopIPFamily(hops.IPFamilies, i)
		if family == model.HopIPAuto {
			continue
		}
		args := map[string]any{"hop": i, "from": edge[0], "to": edge[1], "family": hopFamilyLabel(family)}
		if m.hopDestination(servers[edge[1]].set, family) == nil {
			return invalidCode("err.multiHopIPDestination", "На участке {{from}} → {{to}} нет адреса {{family}} у следующего сервера.", args)
		}
		if !protected && len(m.hopSources(servers[edge[0]], family)) == 0 {
			return invalidCode("err.multiHopIPSource", "На участке {{from}} → {{to}} неизвестен исходящий адрес {{family}}. Укажите адрес ноды или подключите её к мастеру по этому семейству IP.", args)
		}
	}
	return nil
}

func normalizedHopFamilies(families []model.HopIPFamily, count int) ([]model.HopIPFamily, error) {
	if len(families) == 0 {
		return nil, nil
	}
	if len(families) != count {
		return nil, invalidCode("err.multiHopIPFamilies", "Укажите «Авто», IPv4 или IPv6 для каждого участка маршрута.")
	}
	normalized := slices.Clone(families)
	allAuto := true
	for i := range normalized {
		family := hopIPFamily(normalized, i)
		switch family {
		case model.HopIPAuto:
			normalized[i] = family
		case model.HopIPv4, model.HopIPv6:
			allAuto = false
		default:
			return nil, invalidCode("err.multiHopIPFamilies", "Укажите «Авто», IPv4 или IPv6 для каждого участка маршрута.")
		}
	}
	if allAuto {
		return nil, nil
	}
	return normalized, nil
}

func hopIPFamily(families []model.HopIPFamily, i int) model.HopIPFamily {
	if i >= len(families) || families[i] == "" {
		return model.HopIPAuto
	}
	return families[i]
}

func hopFamilyLabel(family model.HopIPFamily) string {
	if family == model.HopIPv6 {
		return "IPv6"
	}
	return "IPv4"
}

func matchesHopFamily(addr netip.Addr, family model.HopIPFamily) bool {
	addr = addr.Unmap()
	return addr.Is4() && family == model.HopIPv4 || addr.Is6() && family == model.HopIPv6
}

// hopDestination fixes the dial address while retaining the TLS identity.
func (m *Manager) hopDestination(set *model.Settings, family model.HopIPFamily) *model.Settings {
	if family == model.HopIPAuto {
		return set
	}
	if ip, err := netip.ParseAddr(set.Host); err == nil {
		if !matchesHopFamily(ip, family) {
			return nil
		}
		return set
	}
	var chosen netip.Addr
	for _, ip := range m.hosts.lookup(set.Host) {
		if matchesHopFamily(ip, family) && (!chosen.IsValid() || ip.Less(chosen)) {
			chosen = ip
		}
	}
	if !chosen.IsValid() {
		return nil
	}
	copy := *set
	if copy.SNI == "" {
		copy.SNI = set.Host
	}
	copy.Host = chosen.String()
	return &copy
}

func (m *Manager) hopSources(server cascadeServer, family model.HopIPFamily) []string {
	sources := m.entryAddrs(server)
	if family == model.HopIPAuto {
		return sources
	}
	return slices.DeleteFunc(sources, func(source string) bool {
		addr, err := netip.ParseAddr(source)
		return err != nil || !matchesHopFamily(addr, family)
	})
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
	family := hopIPFamily(hops.IPFamilies, i-1)
	bridge := xray.Bridge{
		Entry: from, Email: email, UUID: uuid, Lane: model.BridgeVision, Sources: m.hopSources(servers[from], family),
	}
	peer := m.hopDestination(servers[to].set, family)
	if family == model.HopIPAuto {
		peer = m.overIPv4(peer)
	} else if peer == nil || len(bridge.Sources) == 0 {
		return bridge, ""
	}
	return bridge, bridgeLink(model.BridgeVision, model.User{UUID: uuid}, peer)
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
