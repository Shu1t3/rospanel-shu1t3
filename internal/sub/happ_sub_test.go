package sub

import (
	"strings"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/happ"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestHappNodesInSubscription(t *testing.T) {
	u := model.User{ID: 1, Name: "Alice", UUID: "11111111-1111-1111-1111-111111111111", Password: "pass"}
	local := testSet("panel.example.com")

	hn1 := &happ.Node{
		ID:       101,
		Name:     "Happ NL Amsterdam",
		Protocol: "vless",
		Host:     "nl.happ.com",
		Port:     443,
		Enabled:  true,
		URI:      "vless://22222222-2222-2222-2222-222222222222@nl.happ.com:443?type=tcp&security=reality&pbk=1234567890abcdef1234567890abcdef1234567890ab&sni=yahoo.com#Happ%20NL%20Amsterdam",
	}
	hn2 := &happ.Node{
		ID:       102,
		Name:     "Happ DE Frankfurt",
		Protocol: "hysteria2",
		Host:     "de.happ.com",
		Port:     443,
		Enabled:  true,
		URI:      "hysteria2://hypass@de.happ.com:443?sni=de.happ.com#Happ%20DE%20Frankfurt",
	}
	hnDisabled := &happ.Node{
		ID:       103,
		Name:     "Happ Disabled",
		Protocol: "trojan",
		Host:     "off.happ.com",
		Port:     443,
		Enabled:  false,
		URI:      "trojan://trojanpass@off.happ.com:443#Off",
	}

	happNodes := []*happ.Node{hn1, hn2, hnDisabled}

	// 1. Unrestricted access: both enabled Happ nodes must appear in links, singbox, clash
	servers := ServersWithHapp([]*model.Settings{local}, nil, happNodes, model.UnrestrictedAccess())
	links := ShareLinksAll(u, servers)

	var hasNL, hasDE, hasOff bool
	for _, l := range links {
		if strings.Contains(l, "nl.happ.com") {
			hasNL = true
		}
		if strings.Contains(l, "de.happ.com") {
			hasDE = true
		}
		if strings.Contains(l, "off.happ.com") {
			hasOff = true
		}
	}
	if !hasNL || !hasDE {
		t.Errorf("expected enabled Happ nodes in links, got %v", links)
	}
	if hasOff {
		t.Errorf("disabled Happ node should not appear in links")
	}

	// Sing-box check
	singboxJSON := SingBoxJSONMulti(u, servers)
	if !strings.Contains(singboxJSON, "Happ NL Amsterdam") || !strings.Contains(singboxJSON, "Happ DE Frankfurt") {
		t.Errorf("SingBox config missing Happ nodes: %s", singboxJSON)
	}

	// Clash check
	clashYAML := ClashYAMLMulti(u, servers)
	if !strings.Contains(clashYAML, "Happ NL Amsterdam") || !strings.Contains(clashYAML, "Happ DE Frankfurt") {
		t.Errorf("Clash config missing Happ nodes: %s", clashYAML)
	}

	// 2. Restricted access via Group: user only has grant for hn1 (happ:101)
	access := model.Access{
		All: false,
		Tokens: map[string]bool{
			model.HappToken(101): true,
		},
	}
	serversRestricted := ServersWithHapp([]*model.Settings{local}, nil, happNodes, access)
	linksRestricted := ShareLinksAll(u, serversRestricted)

	var hasRestrictedNL, hasRestrictedDE bool
	for _, l := range linksRestricted {
		if strings.Contains(l, "nl.happ.com") {
			hasRestrictedNL = true
		}
		if strings.Contains(l, "de.happ.com") {
			hasRestrictedDE = true
		}
	}
	if !hasRestrictedNL {
		t.Errorf("expected granted Happ node 101 to appear")
	}
	if hasRestrictedDE {
		t.Errorf("un-granted Happ node 102 should NOT appear")
	}
}
