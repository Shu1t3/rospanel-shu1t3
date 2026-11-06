package xray

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Shu1t3/rospanel-shu1t3/internal/hostfw"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// ReverseChain is the part of one protected route installed on this server.
// Entry and Exit may be set together. Relay and Transit are separate servers.
type ReverseChain struct {
	Tag     string
	Entry   *ReverseChainEntry
	Relay   *ReverseChainRelay
	Transit *ReverseChainTransit
	Exit    *ReverseChainExit
	Policy  *hostfw.ReversePolicy
}

type ReverseChainPeer struct {
	Address string
	Port    int
	SNI     string
	Pin     string
	UUID    string
}

type ReverseChainEntry struct {
	Inbound string
	Relay   ReverseChainPeer
	Transit ReverseChainPeer
	BlockIP []string
}

type ReverseChainRelay struct {
	Port        int
	EntryUUID   string
	ExitUUID    string
	TransitIP   string
	TransitPort int
}

type ReverseChainTransit struct {
	Port        int
	TrafficUUID string
	TunnelUUID  string
	Sources     []string
}

type ReverseChainExit struct {
	Relay   ReverseChainPeer
	Tunnel  ReverseChainPeer
	BlockIP []string
}

type VLESSReverse struct {
	Tag string `json:"tag"`
}

func reverseChainListener(tag string, port int, cert []Certificate, clients ...VLESSClient) Inbound {
	return Inbound{
		Tag: tag, Listen: "::", Port: port, Protocol: "vless",
		Settings: VLESSInboundSettings{Clients: clients, Decryption: "none"},
		// No sniffing here. R must check the literal destination of the outer
		// request and must never replace it with the SNI of the inner TLS session.
		StreamSettings: &StreamSettings{
			Network: "tcp", Security: "tls",
			TLSSettings: &TLSSettings{MinVersion: "1.3", Certificates: cert},
		},
	}
}

func reverseChainOutbound(tag string, peer ReverseChainPeer, dialer, reverse string) Outbound {
	settings := map[string]any{
		"address": peer.Address, "port": peer.Port, "id": peer.UUID, "encryption": "none",
	}
	if reverse != "" {
		settings["reverse"] = VLESSReverse{Tag: reverse}
	}
	tls := map[string]any{"serverName": peer.SNI, "minVersion": "1.3"}
	if peer.Pin != "" {
		tls["pinnedPeerCertSha256"] = peer.Pin
	}
	stream := map[string]any{"network": "tcp", "security": "tls", "tlsSettings": tls}
	return Outbound{Tag: tag, Protocol: "vless", Settings: settings, StreamSettings: withDialer(stream, dialer)}
}

// The relay rules precede the ordinary private-address floor. They can reach
// exactly one private endpoint through a dedicated inbound, even when the rest
// of the server forbids private destinations. The entry and exit rules remain
// below the ordinary block rules.
func reverseChainConfig(chains []ReverseChain, cert []Certificate, rc model.RoutingConfig) (in []Inbound, out []Outbound, relay, rules []RouteRule) {
	for _, chain := range chains {
		tag := chain.Tag
		rule := func(suffix, from, to string) RouteRule {
			return RouteRule{Type: "field", RuleTag: tag + suffix, InboundTag: []string{from}, OutboundTag: to}
		}
		if entry := chain.Entry; entry != nil {
			dialer, transit := tag+"-entry-relay", tag+"-transit"
			out = append(out, reverseChainOutbound(dialer, entry.Relay, "", ""), reverseChainOutbound(transit, entry.Transit, dialer, ""))
			rules = append(rules, rule("-entry-route", entry.Inbound, transit))
		}
		if r := chain.Relay; r != nil {
			listener, forward := tag+"-relay-in", tag+"-relay-forward"
			entryEmail, exitEmail := "bridge-"+tag+"-a", "bridge-"+tag+"-c"
			in = append(in, reverseChainListener(listener, r.Port, cert,
				VLESSClient{ID: r.EntryUUID, Email: entryEmail}, VLESSClient{ID: r.ExitUUID, Email: exitEmail}))
			port := strconv.Itoa(r.TransitPort)
			out = append(out, Outbound{Tag: forward, Protocol: "freedom", Settings: map[string]any{
				"redirect":   net.JoinHostPort(r.TransitIP, port),
				"finalRules": []map[string]any{{"action": "allow", "ip": []string{r.TransitIP}, "network": "tcp", "port": port}},
			}})
			allow := rule("-relay-allow", listener, forward)
			allow.IP, allow.Port, allow.Network, allow.User = []string{r.TransitIP}, port, "tcp", []string{entryEmail, exitEmail}
			relay = append(relay, allow, rule("-relay-deny", listener, "block"))
		}
		if b := chain.Transit; b != nil {
			listener, reverse := tag+"-transit-in", tag+"-reverse-out"
			email := "bridge-" + tag + "-transit"
			in = append(in, reverseChainListener(listener, b.Port, cert,
				VLESSClient{ID: b.TrafficUUID, Email: email},
				VLESSClient{ID: b.TunnelUUID, Email: "bridge-" + tag + "-register", Reverse: &VLESSReverse{Tag: reverse}}))
			// Reverse registrations are handled within VLESS, before routing.
			// The listener must also be protected by the host firewall.
			if len(b.Sources) > 0 {
				forward := rule("-transit-route", listener, reverse)
				forward.User, forward.Source = []string{email}, b.Sources
				rules = append(rules, forward)
			}
			rules = append(rules, rule("-transit-deny", listener, "block"))
		}
		if c := chain.Exit; c != nil {
			dialer, reverse, direct := tag+"-exit-relay", tag+"-reverse-in", tag+"-exit-direct"
			out = append(out, reverseChainOutbound(dialer, c.Relay, "", ""),
				reverseChainOutbound(tag+"-register", c.Tunnel, dialer, reverse),
				Outbound{Tag: direct, Protocol: "freedom", Settings: map[string]any{
					"domainStrategy": rc.DirectStrategy,
					"finalRules": []map[string]any{
						{"action": "block", "ip": append(append([]string{}, model.PrivateEgressCIDRs...), c.BlockIP...), "blockDelay": "0"},
						{"action": "allow", "network": "tcp,udp", "ip": []string{"0.0.0.0/0", "::/0"}},
					},
				}})
			rules = append(rules, rule("-exit-route", reverse, direct))
		}
	}
	return in, out, relay, rules
}

// ReverseChainTag scopes each listener, credential and reverse portal to a
// profile revision, so a new topology cannot use a stale reverse registration.
func ReverseChainTag(id int64, revision string) string {
	return fmt.Sprintf("protected-hop-%d-%s", id, revision)
}

// ReverseNetworkPolicy is derived from the same role definitions as Xray.
// A nil policy omits the extension for nodes that have no protected routes.
func ReverseNetworkPolicy(chains []ReverseChain) *hostfw.ReversePolicy {
	var policy hostfw.ReversePolicy
	for _, chain := range chains {
		if p := chain.Policy; p != nil {
			policy.BlockIP = append(policy.BlockIP, p.BlockIP...)
			for port, sources := range p.Transit {
				if policy.Transit == nil {
					policy.Transit = make(map[int][]string)
				}
				policy.Transit[port] = append(policy.Transit[port], sources...)
			}
		}
	}
	if len(policy.Transit) == 0 && len(policy.BlockIP) == 0 {
		return nil
	}
	return &policy
}
