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
