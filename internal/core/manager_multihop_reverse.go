package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"reflect"
	"slices"

	"github.com/Shu1t3/rospanel-shu1t3/internal/hostfw"
	"github.com/Shu1t3/rospanel-shu1t3/internal/ipblock"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func (m *Manager) reverseCapable(server int64) bool {
	if server == model.LocalNodeID {
		return ipblock.CanEnforce()
	}
	_, ok := m.reverseCapabilities.Load(server)
	return ok
}

func (m *Manager) protectedHop(in model.Inbound, server int64, master *model.Settings, servers map[int64]cascadeServer, entry, exit bool) xray.ReverseChain {
	hops := in.Opts.MultiHop
	r := hops.Reverse
	if r == nil || !hops.ValidChain(in.ServerID) || !m.reverseCapable(server) || master.RealityPrivateKey == "" {
		return xray.ReverseChain{}
	}
	nodes := hops.Chain(in.ServerID)
	a, relay, transit, c := nodes[0], nodes[1], nodes[2], nodes[3]
	if !slices.Contains(nodes, server) {
		return xray.ReverseChain{}
	}
	tag := xray.ReverseChainTag(in.ID, hops.Revision)
	hop := xray.ReverseChain{Tag: tag}
	blocked := r.BlockedTransitIPs()
	if set := servers[transit].set; set != nil {
		if ip, err := netip.ParseAddr(set.Host); err == nil {
			if addr := ip.Unmap().String(); !slices.Contains(blocked, addr) {
				blocked = append(blocked, addr)
			}
		}
	}
	// Keep blocking B even when a peer is disabled or its TLS is unavailable.
	// Those changes remove the data plane, but the profile is still protected.
	if server == transit {
		hop.Policy = &hostfw.ReversePolicy{Transit: map[int][]string{r.TransitPort: r.RelaySources}}
	}
	if server == a && entry || server == c && exit {
		hop.Policy = &hostfw.ReversePolicy{BlockIP: blocked}
	}
	for _, id := range nodes {
		if servers[id].set == nil || servers[id].set.Host == "" {
			return hop
		}
	}
	for _, id := range []int64{relay, transit} {
		set := servers[id].set
		if set.TLSInsecure && set.TLSPinSHA256 == "" {
			return hop
		}
	}
	creds := func(role string) string {
		id, _ := serviceCreds(master, tag+"/"+role)
		return id
	}
	peer := func(id int64, address string, port int, uuid string) xray.ReverseChainPeer {
		set := servers[id].set
		return xray.ReverseChainPeer{Address: address, Port: port, SNI: set.Host, Pin: set.TLSPinSHA256, UUID: uuid}
	}
	if server == a && entry {
		hop.Entry = &xray.ReverseChainEntry{Inbound: in.Tag(), BlockIP: blocked,
			Relay:   peer(relay, servers[relay].set.Host, r.RelayPort, creds("a-to-r")),
			Transit: peer(transit, r.TransitIP, r.TransitPort, creds("a-to-b"))}
	}
	if server == relay {
		hop.Relay = &xray.ReverseChainRelay{Port: r.RelayPort, TransitIP: r.TransitIP, TransitPort: r.TransitPort,
			EntryUUID: creds("a-to-r"), ExitUUID: creds("c-to-r")}
	}
	if server == transit {
		hop.Transit = &xray.ReverseChainTransit{Port: r.TransitPort, Sources: r.RelaySources,
			TrafficUUID: creds("a-to-b"), TunnelUUID: creds("c-to-b")}
	}
	if server == c && exit {
		hop.Exit = &xray.ReverseChainExit{BlockIP: blocked,
			Relay:  peer(relay, servers[relay].set.Host, r.RelayPort, creds("c-to-r")),
			Tunnel: peer(transit, r.TransitIP, r.TransitPort, creds("c-to-b"))}
	}
	return hop
}

func protectedHopAck(tag, role string, cfg any) string {
	// These are concrete role structs, whose fields have no custom marshallers.
	data, _ := json.Marshal(cfg)
	return fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%s/%s/%s", tag, role, data)))
}

func addProtectedHop(opts *xray.Options, hop xray.ReverseChain, staged bool) {
	if hop.Entry == nil && hop.Exit == nil && hop.Relay == nil && hop.Transit == nil && hop.Policy == nil {
		return
	}
	opts.ReverseChains = append(opts.ReverseChains, hop)
	if !staged {
		return
	}
	if hop.Entry != nil {
		opts.MultiHopAcks = append(opts.MultiHopAcks, protectedHopAck(hop.Tag, "a", hop.Entry))
	}
	if hop.Relay != nil {
		opts.MultiHopAcks = append(opts.MultiHopAcks, protectedHopAck(hop.Tag, "r", hop.Relay))
	}
	if hop.Transit != nil {
		opts.MultiHopAcks = append(opts.MultiHopAcks, protectedHopAck(hop.Tag, "b", hop.Transit))
	}
	if hop.Exit != nil {
		opts.MultiHopAcks = append(opts.MultiHopAcks, protectedHopAck(hop.Tag, "c", hop.Exit))
	}
}

func (m *Manager) advanceProtectedHop(in model.Inbound, staged model.Inbound, master *model.Settings, servers map[int64]cascadeServer) error {
	pending := in.Opts.MultiHop.Pending
	nodes := pending.Chain()
	if !pending.Reverse.Valid(nodes) {
		return nil
	}
	a, relay, transit, c := nodes[0], nodes[1], nodes[2], nodes[3]
	r := m.protectedHop(staged, relay, master, servers, false, false)
	b := m.protectedHop(staged, transit, master, servers, false, false)
	if r.Relay == nil || b.Transit == nil ||
		!m.hasMultiHopApplied(relay, protectedHopAck(r.Tag, "r", r.Relay)) ||
		!m.hasMultiHopApplied(transit, protectedHopAck(b.Tag, "b", b.Transit)) {
		return nil
	}
	if pending.Ready == 0 {
		pending.Ready = 1
		if err := m.store.UpdateInbound(in); err != nil {
			return err
		}
		m.wakeMultiHopNode(c)
		return nil
	}
	exit := m.protectedHop(staged, c, master, servers, false, true)
	if exit.Exit == nil || !m.hasMultiHopApplied(c, protectedHopAck(exit.Tag, "c", exit.Exit)) {
		return nil
	}
	if !pending.ApplyEntry {
		pending.ApplyEntry = true
		if err := m.store.UpdateInbound(in); err != nil {
			return err
		}
		m.wakeMultiHopNode(a)
		return nil
	}
	entry := m.protectedHop(staged, a, master, servers, true, false)
	if entry.Entry == nil || !m.hasMultiHopApplied(a, protectedHopAck(entry.Tag, "a", entry.Entry)) {
		return nil
	}
	return m.promoteMultiHop(in, staged)
}

// validateProtectedPorts prevents overlapping listeners across live and staged
// revisions. Reserve two unused ports when moving an enabled protected route.
func (m *Manager) validateProtectedPorts(ctx context.Context, in model.Inbound) error {
	hops := in.Opts.MultiHop
	if !in.Enabled || hops == nil || hops.Reverse == nil {
		return nil
	}
	nodes := hops.Chain(in.ServerID)
	routes, err := m.store.MultiHopInbounds(true)
	if err != nil {
		return err
	}
	for _, role := range []struct {
		index, port int
	}{{1, hops.Reverse.RelayPort}, {2, hops.Reverse.TransitPort}} {
		server, port := nodes[role.index], role.port
		set, err := m.effectiveSettings(server)
		if err != nil {
			return err
		}
		ports := reservedPorts(set)
		m.holdPanelPort(ports)
		ports.HoldTCP(set.VLESSPort, "VLESS")
		if _, taken := ports.OnTCP(port); taken {
			return protectedPortError(server, port)
		}
		list, err := m.store.Inbounds(server)
		if err != nil {
			return err
		}
		for _, other := range list {
			if other.Enabled && other.Port == port && portNetwork(other) == "tcp" {
				return protectedPortError(server, port)
			}
		}
		reuse := false
		for _, other := range routes {
			o := other.Opts.MultiHop
			if other.ID == in.ID && o.Pending == nil && reflect.DeepEqual(o.Reverse, hops.Reverse) &&
				slices.Equal(nodes, o.Chain(other.ServerID)) {
				reuse = true
				continue
			}
			if protectedPortUsed(o.Reverse, o.Chain(other.ServerID), server, port) ||
				o.Pending != nil && protectedPortUsed(o.Pending.Reverse, o.Pending.Chain(), server, port) {
				return protectedPortError(server, port)
			}
		}
		if !reuse {
			if err := m.probePort(ctx, server, "tcp", port); err != nil {
				return err
			}
		}
	}
	return nil
}

func protectedPortUsed(route *model.MultiHopReverse, nodes []int64, server int64, port int) bool {
	return route != nil && len(nodes) == 4 &&
		(nodes[1] == server && route.RelayPort == port || nodes[2] == server && route.TransitPort == port)
}

func protectedPortError(server int64, port int) error {
	return invalidCode("err.multiHopReversePort", "TCP-порт {{port}} на сервере {{server}} уже занят. Выберите свободный служебный порт", map[string]any{"port": port, "server": server})
}

func (m *Manager) holdProtectedPorts(server int64, ports model.ReservedPorts) error {
	routes, err := m.store.MultiHopInbounds(true)
	if err != nil {
		return err
	}
	hold := func(r *model.MultiHopReverse, nodes []int64) {
		if r == nil || len(nodes) != 4 {
			return
		}
		if nodes[1] == server {
			ports.HoldTCP(r.RelayPort, "Multi-hop R")
		}
		if nodes[2] == server {
			ports.HoldTCP(r.TransitPort, "Multi-hop B")
		}
	}
	for _, in := range routes {
		r := in.Opts.MultiHop
		hold(r.Reverse, r.Chain(in.ServerID))
		if r.Pending != nil {
			hold(r.Pending.Reverse, r.Pending.Chain())
		}
	}
	return nil
}
