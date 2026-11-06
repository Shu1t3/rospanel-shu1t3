package model

import (
	"net/netip"
	"slices"
)

// MultiHopReverse describes the dedicated TLS ports and the addresses seen
// on the R → B leg. TransitIPs lists B's additional IPv4/IPv6 addresses.
type MultiHopReverse struct {
	Route        []int64  `json:"route,omitempty"`
	RelayPort    int      `json:"relay_port"`
	TransitPort  int      `json:"transit_port"`
	TransitIP    string   `json:"transit_ip"`
	TransitIPs   []string `json:"transit_ips,omitempty"`
	RelaySources []string `json:"relay_sources"`
}

func (r *MultiHopReverse) Clone() *MultiHopReverse {
	if r == nil {
		return nil
	}
	cp := *r
	cp.Route = slices.Clone(r.Route)
	cp.TransitIPs = slices.Clone(r.TransitIPs)
	cp.RelaySources = slices.Clone(r.RelaySources)
	return &cp
}

func (r MultiHopReverse) Valid(nodes []int64) bool {
	if len(nodes) != 4 || !validDistinctMultiHopNodes(nodes[:3]) || nodes[3] < 0 ||
		nodes[3] == nodes[1] || nodes[3] == nodes[2] || nodes[2] == LocalNodeID ||
		r.RelayPort < 1 || r.RelayPort > 65535 || r.TransitPort < 1 || r.TransitPort > 65535 ||
		len(r.RelaySources) == 0 {
		return false
	}
	blocked := make(map[netip.Addr]bool, 1+len(r.TransitIPs))
	for _, raw := range r.BlockedTransitIPs() {
		ip, err := netip.ParseAddr(raw)
		if err != nil || ip.Zone() != "" || !ip.IsGlobalUnicast() {
			return false
		}
		blocked[ip.Unmap()] = true
	}
	for _, raw := range r.RelaySources {
		ip, err := netip.ParseAddr(raw)
		if err != nil || ip.Zone() != "" || !ip.IsGlobalUnicast() || blocked[ip.Unmap()] {
			return false
		}
	}
	return true
}

func (r MultiHopReverse) BlockedTransitIPs() []string {
	return append([]string{r.TransitIP}, r.TransitIPs...)
}
