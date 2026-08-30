package sub

import (
	"github.com/Shu1t3/rospanel-shu1t3/internal/happ"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// Server is one server as a subscription sees it: its effective settings (host, SNI,
// ports, REALITY material, node label), its enabled custom inbounds, imported Happ nodes, and the
// requesting user's access to this server's lanes.
type Server struct {
	Set       *model.Settings
	Custom    []model.Inbound
	HappNodes []*happ.Node
	Access    model.Access
}

// allowsBuiltin / allowsInbound / allowsHapp apply the user's access for THIS server.
func (s Server) allowsBuiltin(lane string) bool {
	return s.Access.AllowsBuiltin(s.Set.ServerID, lane)
}
func (s Server) allowsInbound(id int64) bool { return s.Access.AllowsInbound(id) }
func (s Server) allowsHapp(nodeID int64) bool { return s.Access.AllowsHapp(nodeID) }

// Servers pairs each settings value with its server's custom inbounds and the
// requesting user's access, applied to every server.
func Servers(sets []*model.Settings, custom map[int64][]model.Inbound, access model.Access) []Server {
	return ServersWithHapp(sets, custom, nil, access)
}

// ServersWithHapp pairs settings with custom inbounds, enabled Happ nodes (on master), and access grants.
func ServersWithHapp(sets []*model.Settings, custom map[int64][]model.Inbound, happNodes []*happ.Node, access model.Access) []Server {
	out := make([]Server, 0, len(sets))
	for i, set := range sets {
		isRented := set != nil && set.IsRented
		var hNodes []*happ.Node
		if i == 0 && !isRented {
			hNodes = happNodes
		}
		out = append(out, Server{
			Set:       set,
			Custom:    filterInbounds(custom[set.ServerID], isRented),
			HappNodes: hNodes,
			Access:    access,
		})
	}
	return out
}

// One wraps a single server with no custom inbounds and unrestricted access — the
// shape every legacy single-server helper and test needs.
func One(set *model.Settings) []Server {
	return []Server{{Set: set, Access: model.UnrestrictedAccess()}}
}

// filterInbounds filters out inbounds the operator has switched off, and isolates
// tenant inbounds from owner subscriptions so credentials never mix.
func filterInbounds(list []model.Inbound, isRentedServer bool) []model.Inbound {
	out := make([]model.Inbound, 0, len(list))
	for _, in := range list {
		if !in.Enabled {
			continue
		}
		// On an owner server, do not leak tenant-owned inbounds into owner subscriptions.
		if !isRentedServer && in.IsRental() {
			continue
		}
		out = append(out, in)
	}
	return out
}
