package sub

import (
	"strings"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// A subscription built with external servers: what reaches the user is exactly
// the external servers their access allows, in every format, operating independently
// of physical nodes.
func TestExternalServersFollowAccess(t *testing.T) {
	set := &model.Settings{Host: "1.2.3.4", SNI: "1.2.3.4", ServerID: model.LocalNodeID, SubShowConfigs: true}
	ext := []model.ExtServer{
		{ID: 11, Name: "Partner NL", Protocol: "vless", Host: "9.9.9.9", Port: 443, Enabled: true,
			Link: "vless://uuid@9.9.9.9:443?type=tcp&security=tls&sni=nl.example&fp=chrome#Partner%20NL"},
		{ID: 12, Name: "Partner DE", Protocol: "hysteria2", Host: "8.8.8.8", Port: 443, Enabled: true,
			Link: "hysteria2://pw@8.8.8.8:443?sni=de.example#Partner%20DE"},
		{ID: 13, Name: "Off", Protocol: "trojan", Host: "7.7.7.7", Port: 443, Enabled: false,
			Link: "trojan://pw@7.7.7.7:443?security=tls&sni=x#Off"},
	}
	u := model.User{ID: 1, UUID: "u", Password: "p"}

	unrestricted := model.UnrestrictedAccess()
	req := Request{
		User:     u,
		Settings: set,
		External: ext,
		Access:   unrestricted,
	}
	links := GenerateShareLinks(req)
	if len(links) != 2 || links[0] != ext[0].Link || links[1] != ext[1].Link {
		t.Fatalf("unrestricted links: %v", links)
	}
	if yaml := GenerateClash(req); !strings.Contains(yaml, `"Partner NL"`) || !strings.Contains(yaml, `"Partner DE"`) || strings.Contains(yaml, `"Off"`) {
		t.Fatalf("clash: %s", yaml)
	}
	if sb := GenerateSingBox(req); !strings.Contains(sb, `"Partner NL"`) || !strings.Contains(sb, `"hysteria2"`) {
		t.Fatalf("sing-box: %s", sb)
	}
	if xj := GenerateXrayJSON(req); !strings.Contains(xj, `"remarks": "Partner NL"`) || !strings.Contains(xj, `"protocol": "hysteria"`) {
		t.Fatalf("xray json: %s", xj)
	}

	// HTML page renders external links even when no physical servers exist
	html, err := GeneratePage(req, Billing{}, Devices{}, true, "en")
	if err != nil {
		t.Fatalf("page without physical servers failed: %v", err)
	}
	if !strings.Contains(string(html), "Partner NL") {
		t.Fatalf("page missing external server link: %s", string(html))
	}

	// A group that grants one of them.
	one := model.Access{Tokens: map[string]bool{model.ExtToken(12): true}}
	reqOne := Request{User: u, Settings: set, External: ext, Access: one}
	if links = GenerateShareLinks(reqOne); len(links) != 1 || links[0] != ext[1].Link {
		t.Fatalf("restricted links: %v", links)
	}
	none := model.Access{Tokens: map[string]bool{}}
	reqNone := Request{User: u, Settings: set, External: ext, Access: none}
	if links = GenerateShareLinks(reqNone); len(links) != 0 {
		t.Fatalf("no grants: %v", links)
	}
}

// A REALITY lane is only handed out once the panel has minted its keys: the public
// key is what a client authenticates the handshake with, and a link without one
// fails with nothing a user could act on. A node added before its keys landed used
// to produce exactly that link.
func TestRealityLaneNeedsItsPublicKey(t *testing.T) {
	set := &model.Settings{
		Host: "1.2.3.4", SNI: "1.2.3.4", ServerID: model.LocalNodeID,
		RealityEnabled: true, RealityPort: 8443, RealityDest: "max.ru",
		RealityShortID: "ab", RealityPath: "/x",
	}
	u := model.User{ID: 1, UUID: "uuid", Password: "pw"}
	srv := Server{Set: set, Access: model.UnrestrictedAccess()}

	if links := ShareLinks(u, srv); len(links) != 0 {
		t.Fatalf("a keyless REALITY lane was handed out: %v", links)
	}
	if yaml := ClashYAMLMulti(u, []Server{srv}); strings.Contains(yaml, "reality-opts") {
		t.Error("a keyless REALITY proxy reached the Clash profile")
	}

	set.RealityPublicKey = "PUBKEY"
	links := ShareLinks(u, srv)
	if len(links) != 1 || !strings.Contains(links[0], "pbk=PUBKEY") {
		t.Fatalf("with a key the lane must be handed out: %v", links)
	}
}
