package sub

import (
	"strings"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestMultiHopSeparateProfileRequiresGrant(t *testing.T) {
	u := model.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}
	in := model.Inbound{ID: 4, Enabled: true, Name: "Multi-hop", Port: 8443, Protocol: model.InbVLESS,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS, MultiHop: &model.MultiHop{Transit: 1, Exit: 2}}}
	in.Normalize()
	set := testSet("entry.example.org")
	srv := Server{Set: set, Custom: []model.Inbound{in}, Access: model.UnrestrictedAccess()}
	if got := strings.Join(ShareLinks(u, srv), "\n"); strings.Contains(got, in.VLESSUUID(u.UUID)) {
		t.Fatal("published multi-hop without a group grant")
	}

	srv.Access = model.Access{Tokens: map[string]bool{
		model.InboundToken(in.ID): true, model.BuiltinToken(set.ServerID, model.LaneVLESS): true,
	}}
	servers := []Server{srv}
	for format, got := range map[string]string{
		"share links": strings.Join(ShareLinks(u, srv), "\n"),
		"sing-box":    SingBoxJSONMulti(u, servers),
		"Clash":       ClashYAMLMulti(u, servers),
	} {
		for _, want := range []string{set.Host, u.UUID, in.VLESSUUID(u.UUID), "8443"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s lacks %q", format, want)
			}
		}
	}
}

func TestIPv6MultiHopSubscriptionAddresses(t *testing.T) {
	set := testSet("2001:db8::42")
	if got, want := URL(set, "token"), "https://[2001:db8::42]/"+set.SubPathOr()+"/token"; got != want {
		t.Fatalf("subscription URL = %q, want %q", got, want)
	}
	in := model.Inbound{ID: 4, Enabled: true, Name: "IPv6", Port: 28443, Protocol: model.InbVLESS,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS, MultiHop: &model.MultiHop{Transit: 1, Exit: 2}}}
	in.Normalize()
	user := model.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}
	server := Server{Set: set, Custom: []model.Inbound{in}, Access: model.Access{Tokens: map[string]bool{model.InboundToken(in.ID): true}}}
	for _, link := range ShareLinks(user, server) {
		if strings.Contains(link, "@[2001:db8::42]:28443?") {
			return
		}
	}
	t.Fatal("subscription did not contain the bracketed IPv6 Multi-Hop entry")
}

func TestProtectedMultiHopKeepsServerTopologyOutOfClientProfiles(t *testing.T) {
	u := model.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}
	in := model.Inbound{
		ID: 4, Enabled: true, Name: "Multi-hop", Port: 18443, Protocol: model.InbVLESS,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS,
			MultiHop: &model.MultiHop{Transit: 1, Exit: 2}},
	}
	in.Normalize()
	set := testSet("entry.example.org")
	access := model.Access{Tokens: map[string]bool{model.InboundToken(in.ID): true}}
	render := func(in model.Inbound) map[string]string {
		server := Server{Set: set, Custom: []model.Inbound{in}, Access: access}
		servers := []Server{server}
		return map[string]string{
			"share links": strings.Join(ShareLinks(u, server), "\n"),
			"Xray JSON":   XrayJSONMulti(u, servers, model.SubDPI{}),
			"sing-box":    SingBoxJSONMulti(u, servers),
			"Clash":       ClashYAMLMulti(u, servers),
		}
	}

	before := render(in)
	in.Opts.MultiHop = &model.MultiHop{Transit: -1, Exit: -1, Reverse: &model.MultiHopReverse{
		Route: []int64{0, 1, 2, 3}, RelayPort: 29443, TransitPort: 29444,
		TransitIP: "192.0.2.13", TransitIPs: []string{"2001:db8::13"}, RelaySources: []string{"192.0.2.12"},
	}}
	for format, got := range render(in) {
		t.Run(format, func(t *testing.T) {
			for _, want := range []string{set.Host, "18443", in.VLESSUUID(u.UUID)} {
				if !strings.Contains(got, want) {
					t.Errorf("protected profile lacks client parameter %q", want)
				}
			}
			if got != before[format] {
				t.Error("switching to the protected route changed the client subscription")
			}
		})
	}
}
