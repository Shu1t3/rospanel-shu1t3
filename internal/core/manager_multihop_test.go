package core

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func TestMultiHopNodeStateFollowsDownstream(t *testing.T) {
	t.Parallel()
	m := nodeTestManager(t)
	if err := m.store.SetRealityKeys("master-priv", "master-pub", "ab", "/rp"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"entry", "transit", "exit"} {
		syncedNode(t, m.store, name, xray.PinnedVersion)
	}
	nodes, err := m.store.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]*model.Node, len(nodes))
	yes := true
	for i := range nodes {
		n := &nodes[i]
		n.VLESSEnabled = &yes
		n.Host = n.Name + ".example.com"
		if err := m.store.UpdateNode(n.ID, store.NodeEdit{Name: n.Name, Host: n.Host, VLESS: &yes}); err != nil {
			t.Fatal(err)
		}
		if err := m.store.UpdateNodeStatus(n.ID, model.NodeStatusUpdate{
			LastSeen: time.Now().Unix(), XrayVersion: xray.PinnedVersion,
			XrayRunning: true, CertSHA256: strings.Repeat("ab", 32),
		}); err != nil {
			t.Fatal(err)
		}
		byName[n.Name] = n
	}
	entry, transit, exit := byName["entry"], byName["transit"], byName["exit"]
	in := model.Inbound{
		ServerID: entry.ID, Enabled: true, Name: "Multi-hop", Protocol: model.InbVLESS, Port: 8443,
		Opts: model.InboundOpts{
			Transport: model.TrTCP, Security: model.SecTLS,
			MultiHop: &model.MultiHop{Transit: transit.ID, Exit: exit.ID},
		},
	}
	in.Normalize()
	if err := m.validateMultiHop(in); err != nil {
		t.Fatal(err)
	}
	saved, err := m.store.CreateInbound(in)
	if err != nil {
		t.Fatal(err)
	}

	entryState, err := m.NodeDesiredState(entry)
	if err != nil {
		t.Fatal(err)
	}
	entryTag := "multi-hop-" + saved.Tag()
	if !bytes.Contains(entryState.XrayConfig, []byte(transit.Host)) ||
		!bytes.Contains(entryState.XrayConfig, []byte(entryTag)) ||
		bytes.Contains(entryState.XrayConfig, []byte(exit.Host)) {
		t.Fatalf("entry should dial only the transit")
	}
	transitState, err := m.NodeDesiredState(transit)
	if err != nil {
		t.Fatal(err)
	}
	bridge := fmt.Sprintf("bridge-multi-hop-%d-transit", saved.ID)
	if !bytes.Contains(transitState.XrayConfig, []byte(exit.Host)) ||
		!bytes.Contains(transitState.XrayConfig, []byte(bridge)) {
		t.Fatal("transit should accept the entry bridge and dial the exit")
	}
	exitState, err := m.NodeDesiredState(exit)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(exitState.XrayConfig, []byte(`"outboundTag":"multi-hop-exit"`)) ||
		!bytes.Contains(exitState.XrayConfig, []byte(`"finalRules"`)) {
		t.Fatal("exit should apply the dedicated egress rule")
	}

	// Rotating an exit address invalidates the transit's remembered state without
	// changing the credential or the address published to the client.
	exit.Host = "new-exit.example.com"
	if err := m.store.UpdateNode(exit.ID, store.NodeEdit{Name: exit.Name, Host: exit.Host, VLESS: &yes}); err != nil {
		t.Fatal(err)
	}
	moved, err := m.NodeStateChange(transit, transitState.Hash)
	if err != nil || moved == nil || !bytes.Contains(moved.XrayConfig, []byte(exit.Host)) {
		t.Fatalf("transit did not follow the new exit: %v", err)
	}
	set := mustSettings(t, m)
	opts, err := m.genOptsFor(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	l, err := url.Parse(opts.MultiHopLinks[saved.ID])
	if err != nil || l.Hostname() != transit.Host {
		t.Fatalf("entry link should stay on the transit: %v", err)
	}
	if strings.Contains(l.String(), set.RealityPrivateKey) {
		t.Fatal("the key itself appeared in the bridge link")
	}

	no := false
	if err := m.store.UpdateNode(exit.ID, store.NodeEdit{Name: exit.Name, Host: exit.Host, VLESS: &no}); err != nil {
		t.Fatal(err)
	}
	blocked, err := m.NodeStateChange(entry, entryState.Hash)
	if err != nil || blocked == nil ||
		!bytes.Contains(blocked.XrayConfig, []byte(`"inboundTag":["`+saved.Tag()+`"],"outboundTag":"block"`)) ||
		bytes.Contains(blocked.XrayConfig, []byte(transit.Host)) {
		t.Fatalf("a disabled exit must block the entry profile: %v", err)
	}
}
