package core

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"slices"

	"github.com/google/uuid"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// MultiHopProfile is the operator-facing route. Its listener remains a private
// implementation detail, with the same identity and grants as pre-existing routes.
type MultiHopProfile struct {
	ID       int64                  `json:"id"`
	Name     string                 `json:"name"`
	Entry    int64                  `json:"entry"`
	Transit  int64                  `json:"transit"`
	Exit     int64                  `json:"exit"`
	Enabled  bool                   `json:"enabled"`
	Port     int                    `json:"port"`
	Nodes    []int64                `json:"nodes"`
	Applying bool                   `json:"applying,omitempty"`
	Reverse  *model.MultiHopReverse `json:"reverse,omitempty"`
}

func multiHopProfile(in model.Inbound) MultiHopProfile {
	hops := in.Opts.MultiHop
	p := MultiHopProfile{
		ID: in.ID, Name: in.Name, Entry: in.ServerID, Port: in.Port, Enabled: in.Enabled,
		Transit: hops.Transit, Exit: hops.Exit, Nodes: hops.Chain(in.ServerID),
		Applying: hops.Pending != nil, Reverse: hops.Reverse,
	}
	if hops.Pending != nil {
		p.Nodes, p.Name, p.Reverse = hops.Pending.Chain(), hops.Pending.Name, hops.Pending.Reverse
	}
	return p
}

func (m *Manager) MultiHopProfiles() ([]MultiHopProfile, error) {
	inbounds, err := m.store.MultiHopInbounds(false)
	if err != nil {
		return nil, err
	}
	out := make([]MultiHopProfile, 0, len(inbounds))
	for _, in := range inbounds {
		out = append(out, multiHopProfile(in))
	}
	return out, nil
}

func multiHopFromChain(nodes []int64) model.MultiHop {
	via := append([]int64{}, nodes[1:]...)
	hops := model.MultiHop{Via: &via}
	if len(via) == 2 {
		hops.Transit, hops.Exit = via[0], via[1]
	}
	return hops
}

func protectedMultiHop(nodes []int64, reverse *model.MultiHopReverse) model.MultiHop {
	reverse = reverse.Clone()
	reverse.Route = slices.Clone(nodes)
	reverse.TransitIP = netip.MustParseAddr(reverse.TransitIP).Unmap().String()
	reverse.TransitIPs = normalizedProtectedIPs(reverse.TransitIPs)
	reverse.RelaySources = normalizedProtectedIPs(reverse.RelaySources)
	// Older panels see an invalid ordinary chain and leave the entry closed.
	return model.MultiHop{Transit: -1, Exit: -1, Reverse: reverse}
}

func normalizedProtectedIPs(ips []string) []string {
	var normalized []string
	for _, ip := range ips {
		normalized = append(normalized, netip.MustParseAddr(ip).Unmap().String())
	}
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func newMultiHopDeployment(nodes []int64, name string, initial, legacy bool, reverse *model.MultiHopReverse) *model.MultiHopPending {
	p := &model.MultiHopPending{Nodes: slices.Clone(nodes), Name: name, Revision: uuid.NewString(), Initial: initial, Legacy: legacy}
	if reverse != nil {
		p.Reverse = protectedMultiHop(nodes, reverse).Reverse
		p.Nodes = nil // Keep older panels from staging an ordinary chain.
		p.Legacy = false
	}
	return p
}

func (m *Manager) CreateMultiHopChain(ctx context.Context, name string, nodes []int64) (*MultiHopProfile, error) {
	return m.CreateMultiHopRoute(ctx, name, nodes, nil)
}

func (m *Manager) CreateMultiHopRoute(ctx context.Context, name string, nodes []int64, reverse *model.MultiHopReverse) (*MultiHopProfile, error) {
	reverse, err := m.protectedAddresses(nodes, reverse)
	if err != nil {
		return nil, err
	}
	if err := validateMultiHopNodes(nodes, reverse); err != nil {
		return nil, err
	}
	hops := multiHopFromChain(nodes)
	if reverse != nil {
		hops = protectedMultiHop(nodes, reverse)
	}
	if reverse != nil {
		m.multiHopWrites.Lock()
		defer m.multiHopWrites.Unlock()
	}
	return m.CreateMultiHopProfile(ctx, nodes[0], name, hops)
}

// Remember a literal public B address even if B is subsequently disabled or
// deleted. Domains with multiple A/AAAA records still need an explicit list.
func (m *Manager) protectedAddresses(nodes []int64, reverse *model.MultiHopReverse) (*model.MultiHopReverse, error) {
	if reverse == nil || len(nodes) != 4 {
		return reverse, nil
	}
	n, err := m.store.GetNode(nodes[2])
	if err != nil {
		return nil, err
	}
	if n == nil {
		return reverse, nil
	}
	if ip, err := netip.ParseAddr(n.Host); err == nil && ip.IsGlobalUnicast() {
		addr := ip.Unmap()
		for _, raw := range reverse.BlockedTransitIPs() {
			if other, err := netip.ParseAddr(raw); err == nil && addr == other.Unmap() {
				return reverse, nil
			}
		}
		reverse = reverse.Clone()
		reverse.TransitIPs = append(reverse.TransitIPs, addr.String())
	}
	return reverse, nil
}

func validateMultiHopNodes(nodes []int64, reverse *model.MultiHopReverse) error {
	if reverse != nil {
		if !reverse.Valid(nodes) {
			return invalidCode("err.multiHopReverse", "проверьте роли, служебные порты и IP-адреса защищённого маршрута")
		}
	} else if !model.ValidMultiHopChain(nodes) {
		return invalidCode("err.multiHopDistinct", "выберите один или несколько разных серверов для мульти-хопа")
	}
	return nil
}

func (m *Manager) CreateMultiHopProfile(ctx context.Context, entry int64, name string, hops model.MultiHop) (*MultiHopProfile, error) {
	set, err := m.effectiveSettings(entry)
	if err != nil {
		return nil, err
	}
	inbounds, err := m.store.Inbounds(entry)
	if err != nil {
		return nil, err
	}
	ports := reservedPorts(set)
	m.holdPanelPort(ports)
	// Even with no built-in VLESS clients its listener can still serve the decoy.
	ports.HoldTCP(set.VLESSPort, "VLESS")
	for _, in := range inbounds {
		if portNetwork(in) == "tcp" {
			ports.HoldTCP(in.Port, in.Name)
		}
	}
	in := model.Inbound{
		ServerID: entry, Name: name, Protocol: model.InbVLESS, Enabled: true,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS, MultiHop: &hops},
	}
	// Keep automatically assigned listeners in a predictable range. CreateInbound
	// also checks the host and the store's unique index covers racing creates.
	for port := 28443; port < 28507; port++ {
		if _, taken := ports.OnTCP(port); taken {
			continue
		}
		in.Port = port
		saved, err := m.CreateInbound(ctx, in)
		if err == nil {
			profile := multiHopProfile(saved.Inbound)
			return &profile, nil
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			return nil, err
		}
		switch ve.Code {
		case "err.portTakenHere":
			continue
		case "err.portTakenOnServer", "err.portTakenBy", "err.portTakenByInbound":
			if ve.Args["port"] == port {
				continue
			}
		}
		return nil, err
	}
	return nil, invalidCode("err.multiHopPort", "на входном сервере нет свободного TCP-порта в диапазоне 28443–28506")
}

func (m *Manager) UpdateMultiHopChain(ctx context.Context, id int64, name string, nodes []int64, enabled bool) (*MultiHopProfile, error) {
	return m.UpdateMultiHopRoute(ctx, id, name, nodes, enabled, nil)
}

func (m *Manager) UpdateMultiHopRoute(ctx context.Context, id int64, name string, nodes []int64, enabled bool, reverse *model.MultiHopReverse) (*MultiHopProfile, error) {
	reverse, err := m.protectedAddresses(nodes, reverse)
	if err != nil {
		return nil, err
	}
	if err := validateMultiHopNodes(nodes, reverse); err != nil {
		return nil, err
	}
	m.multiHopWrites.Lock()
	defer m.multiHopWrites.Unlock()
	in, err := m.getMultiHopInbound(id)
	if err != nil {
		return nil, err
	}
	old := *in
	hops := in.Opts.MultiHop
	if enabled && hops.Pending != nil && hops.Pending.ApplyEntry {
		return nil, invalidCode("err.multiHopApplying", "дождитесь применения предыдущего изменения маршрута")
	}
	candidate := *in
	newRoute := multiHopFromChain(nodes)
	if reverse != nil {
		newRoute = protectedMultiHop(nodes, reverse)
	}
	if enabled && in.Enabled && (hops.Reverse != nil && reverse == nil ||
		reverse != nil && hops.Reverse == nil && !hops.Provisioning() && slices.Contains(hops.Chain(in.ServerID), nodes[2])) {
		return nil, invalidCode("err.multiHopReverseDisable", "выключите действующий маршрут перед сменой режима")
	}
	candidate.ServerID, candidate.Name, candidate.Enabled = nodes[0], name, enabled
	candidate.Opts.MultiHop = &newRoute
	candidate.Normalize()
	if err := m.validateAgainstSet(ctx, candidate, id); err != nil {
		return nil, err
	}
	switch {
	case !enabled:
		*in = candidate
	case in.Enabled && !hops.Provisioning() && slices.Equal(nodes, hops.Chain(in.ServerID)) && reflect.DeepEqual(newRoute.Reverse, hops.Reverse):
		in.Name = candidate.Name
		hops.Pending = nil
	default:
		in.Enabled = true
		if !old.Enabled {
			in.ServerID, in.Name = candidate.ServerID, candidate.Name
			in.Opts.MultiHop = &newRoute
		}
		in.Opts.MultiHop.Pending = newMultiHopDeployment(nodes, candidate.Name, !old.Enabled || hops.Provisioning(), false, reverse)
	}
	if err := m.store.UpdateMultiHopInbound(*in); err != nil {
		return nil, inboundConflict(err)
	}
	m.applyInboundChange(old.ServerID, true)
	profile := multiHopProfile(*in)
	return &profile, nil
}

func (m *Manager) UpdateMultiHopProfile(ctx context.Context, id int64, name string, hops model.MultiHop, enabled bool) (*MultiHopProfile, error) {
	in, err := m.getMultiHopInbound(id)
	if err != nil {
		return nil, err
	}
	if in.Opts.MultiHop.Via != nil || in.Opts.MultiHop.Reverse != nil {
		return nil, invalidCode("err.multiHopManaged", "управляйте маршрутом в разделе «Мульти-хоп»")
	}
	// The entry's transport, port and ID are part of the client's existing profile.
	in.Name, in.Opts.MultiHop, in.Enabled = name, &hops, enabled
	saved, err := m.UpdateInbound(ctx, *in)
	if err != nil {
		return nil, err
	}
	profile := multiHopProfile(saved.Inbound)
	return &profile, nil
}

func (m *Manager) DeleteMultiHopProfile(id int64) error {
	if _, err := m.getMultiHopInbound(id); err != nil {
		return err
	}
	return m.DeleteInbound(id)
}

func (m *Manager) getMultiHopInbound(id int64) (*model.Inbound, error) {
	in, err := m.store.GetInbound(id)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Opts.MultiHop == nil {
		return nil, invalidCode("err.multiHopNotFound", "профиль мульти-хопа не найден")
	}
	return in, nil
}
