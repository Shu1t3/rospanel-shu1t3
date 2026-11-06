package core

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestProtectedMultiHopStoredTopologyFailsClosedOnOlderPanels(t *testing.T) {
	t.Parallel()
	nodes := []int64{1, 2, 3, 1}
	config := &model.MultiHopReverse{
		RelayPort: 29443, TransitPort: 29444, TransitIP: "192.0.2.3", RelaySources: []string{"192.0.2.2"},
	}
	if !config.Valid(nodes) {
		t.Fatal("combined A/C rejected")
	}
	hops := protectedMultiHop(nodes, config)
	hops.Pending = newMultiHopDeployment(nodes, "protected", true, false, config)
	if !hops.ValidChain(nodes[0]) || !slices.Equal(hops.Chain(nodes[0]), nodes) || !slices.Equal(hops.Pending.Chain(), nodes) {
		t.Fatal("protected topology lost")
	}
	raw, err := json.Marshal(hops)
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Transit, Exit int64
		Via           *[]int64
		Pending       struct{ Nodes []int64 }
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	if old.Via != nil || model.ValidMultiHopChain([]int64{1, old.Transit, old.Exit}) ||
		model.ValidMultiHopChain(old.Pending.Nodes) {
		t.Fatal("old panel can misinterpret a protected route as ordinary")
	}
}
