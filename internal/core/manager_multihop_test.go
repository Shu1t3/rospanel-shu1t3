package core

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/nodeapi"
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
	// A manually configured listener keeps its port. The route command picks
	// the next one and provisions its own VLESS TCP-TLS entry.
	_, err = m.store.CreateInbound(model.Inbound{
		ServerID: entry.ID, Enabled: true, Name: "Ordinary", Protocol: model.InbVLESS, Port: 28443,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := m.CreateMultiHopProfile(context.Background(), entry.ID, "Multi-hop", model.MultiHop{Transit: transit.ID, Exit: exit.ID})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := m.store.GetInbound(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || profile.Port != 28444 || saved.Opts.Security != model.SecTLS {
		t.Fatalf("unexpected generated listener: %+v", profile)
	}

	// Keep a v5.6-format listener as an upgrade fixture. Newly created profiles,
	// including those using the legacy API, deploy through the rollout state machine.
	saved.Opts.MultiHop.Pending = nil
	if err := m.store.UpdateInbound(*saved); err != nil {
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

func TestMultiHopRolloutKeepsPreviousRouteUntilEntryApplies(t *testing.T) {
	t.Parallel()
	t.Run("distinct exit", func(t *testing.T) { testMultiHopRollout(t, false) })
	t.Run("return to entry", func(t *testing.T) { testMultiHopRollout(t, true) })
}

func testMultiHopRollout(t *testing.T, returnToEntry bool) {
	t.Helper()
	m := nodeTestManager(t)
	if err := m.store.SetRealityKeys("master-priv", "master-pub", "ab", "/rp"); err != nil {
		t.Fatal(err)
	}
	yes := true
	var nodes []*model.Node
	for i := 0; i < 6; i++ {
		host := fmt.Sprintf("192.0.2.%d", i+1)
		n, err := m.store.CreateNode(fmt.Sprintf("Hop %d", i), host, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := m.store.UpdateNode(n.ID, store.NodeEdit{Name: n.Name, Host: host, VLESS: &yes}); err != nil {
			t.Fatal(err)
		}
		if err := m.store.UpdateNodeStatus(n.ID, model.NodeStatusUpdate{LastSeen: time.Now().Unix(), XrayRunning: true, XrayVersion: xray.PinnedVersion, CertSHA256: strings.Repeat("ab", 32)}); err != nil {
			t.Fatal(err)
		}
		n, err = m.store.GetNode(n.ID)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}
	old, err := m.store.CreateInbound(model.Inbound{
		ServerID: nodes[0].ID, Port: 28443, Name: "Legacy", Enabled: true, Protocol: model.InbVLESS,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS,
			MultiHop: &model.MultiHop{Transit: nodes[1].ID, Exit: nodes[2].ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	get := func() *model.Inbound {
		t.Helper()
		in, err := m.store.GetInbound(old.ID)
		if err != nil || in == nil {
			t.Fatalf("read backing inbound: %v", err)
		}
		return in
	}
	exit := nodes[5]
	if returnToEntry {
		exit = nodes[1]
	}
	chain := []int64{nodes[1].ID, nodes[3].ID, nodes[2].ID, nodes[4].ID, exit.ID}
	profile, err := m.UpdateMultiHopChain(context.Background(), old.ID, "Updated", chain, true)
	if err != nil || !profile.Applying || !slices.Equal(profile.Nodes, chain) {
		t.Fatalf("save new route: %+v (%v)", profile, err)
	}
	state := func(n *model.Node) *nodeapi.NodeState {
		t.Helper()
		st, err := m.NodeDesiredState(n)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	advance := func() {
		t.Helper()
		if err := m.advanceMultiHop(); err != nil {
			t.Fatal(err)
		}
	}
	ack := func(n *model.Node, want int) {
		t.Helper()
		acks := state(n).Meta.MultiHopAcks
		if len(acks) != want {
			t.Fatalf("%s reports %v, want %d deployment receipts", n.Name, acks, want)
		}
		m.noteMultiHopApplied(n.ID, acks)
	}
	checkOld := func() {
		t.Helper()
		in := get()
		if in.ServerID != old.ServerID || in.Port != old.Port || in.Name != old.Name || in.Opts.MultiHop.Via != nil {
			t.Fatalf("published legacy entry changed before new entry applied: %+v", in)
		}
		cfg := state(nodes[0]).XrayConfig
		if !bytes.Contains(cfg, []byte(nodes[1].Host)) || bytes.Contains(cfg, []byte(nodes[3].Host)) {
			t.Fatal("old entry must continue using the complete old route")
		}
	}
	checkOld()
	advance()
	if get().Opts.MultiHop.Pending.Ready != 0 || len(state(nodes[4]).Meta.MultiHopAcks) != 0 {
		t.Fatal("rollout advanced without an exit receipt")
	}
	ack(exit, 1)
	advance()
	ack(nodes[4], 1)
	advance()
	// After a restart the deployment stays installed, but receipts must be
	// refreshed before switching an entry.
	m.multiHopApplied = nil
	ack(nodes[2], 1)
	advance()
	if get().Opts.MultiHop.Pending.Ready != 2 {
		t.Fatal("rollout advanced without re-confirming the deployed suffix")
	}
	ack(exit, 1)
	ack(nodes[4], 1)
	advance()
	ack(nodes[3], 1)
	advance()
	checkOld()
	if !get().Opts.MultiHop.Pending.ApplyEntry {
		t.Fatal("entry should be staged after all downstream hops have applied")
	}
	entryAcks := 1
	if returnToEntry {
		entryAcks = 2
	}
	newEntry := state(nodes[1])
	if !bytes.Contains(newEntry.XrayConfig, []byte(nodes[3].Host)) || len(newEntry.Meta.MultiHopAcks) != entryAcks {
		t.Fatal("new entry should now be able to apply its VLESS listener")
	}
	advance()
	checkOld()
	ack(nodes[1], entryAcks)
	advance()
	in := get()
	if in.ID != old.ID || in.Port != old.Port || in.ServerID != chain[0] ||
		in.Name != "Updated" || in.Opts.MultiHop.Pending != nil || !slices.Equal(in.Opts.MultiHop.Chain(in.ServerID), chain) ||
		in.VLESSUUID("example") != old.VLESSUUID("example") {
		t.Fatalf("new chain was not published with its original identity: %+v", in)
	}
	if returnToEntry {
		cfg := state(nodes[1]).XrayConfig
		if !bytes.Contains(cfg, []byte(nodes[3].Host)) ||
			!bytes.Contains(cfg, []byte(`"outboundTag":"multi-hop-exit"`)) {
			t.Fatal("published config must keep the entry link and the final egress on the same server")
		}
	}
	if cfg := state(nodes[0]).XrayConfig; bytes.Contains(cfg, []byte(old.Tag())) {
		t.Fatal("old entry still contains the moved listener")
	}
}
