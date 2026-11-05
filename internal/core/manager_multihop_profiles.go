package core

import (
	"context"
	"errors"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// MultiHopProfile is the operator-facing route. Its listener remains a private
// implementation detail, with the same identity and grants as pre-existing routes.
type MultiHopProfile struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Entry   int64  `json:"entry"`
	Transit int64  `json:"transit"`
	Exit    int64  `json:"exit"`
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
}

func multiHopProfile(in model.Inbound) MultiHopProfile {
	return MultiHopProfile{
		ID: in.ID, Name: in.Name, Entry: in.ServerID, Port: in.Port, Enabled: in.Enabled,
		Transit: in.Opts.MultiHop.Transit, Exit: in.Opts.MultiHop.Exit,
	}
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

func (m *Manager) UpdateMultiHopProfile(ctx context.Context, id int64, name string, hops model.MultiHop, enabled bool) (*MultiHopProfile, error) {
	in, err := m.getMultiHopInbound(id)
	if err != nil {
		return nil, err
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
