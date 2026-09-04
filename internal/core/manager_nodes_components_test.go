package core

import (
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/nodeapi"
)

func TestAggregateComponentStatus(t *testing.T) {
	tests := []struct {
		name       string
		components []nodeapi.ComponentStatus
		want       string
	}{
		{
			name:       "empty slice defaults to healthy",
			components: nil,
			want:       "healthy",
		},
		{
			name: "all disabled components evaluate to healthy",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusDisabled},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusDisabled},
			},
			want: "healthy",
		},
		{
			name: "all enabled components healthy",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusHealthy, Running: true},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusHealthy, Running: true},
			},
			want: "healthy",
		},
		{
			name: "one healthy and one disabled is healthy",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusHealthy, Running: true},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusDisabled},
			},
			want: "healthy",
		},
		{
			name: "one healthy and one unhealthy is degraded",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusHealthy, Running: true},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusUnhealthy, Running: false},
			},
			want: "degraded",
		},
		{
			name: "one healthy and one degraded is degraded",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusHealthy, Running: true},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusDegraded, Running: false},
			},
			want: "degraded",
		},
		{
			name: "all enabled components unhealthy is unhealthy",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusUnhealthy, Running: false},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusUnhealthy, Running: false},
			},
			want: "unhealthy",
		},
		{
			name: "unhealthy with disabled is unhealthy",
			components: []nodeapi.ComponentStatus{
				{Name: nodeapi.ComponentXray, Status: nodeapi.StatusUnhealthy, Running: false},
				{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusDisabled},
			},
			want: "unhealthy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AggregateComponentStatus(tc.components)
			if got != tc.want {
				t.Errorf("AggregateComponentStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNodeComponentsAndAggregatedStatus(t *testing.T) {
	mgr := newTestManager(t)

	// Nil node returns unknown
	if st := mgr.NodeAggregatedStatus(nil); st != "unknown" {
		t.Errorf("NodeAggregatedStatus(nil) = %q, want unknown", st)
	}

	// Master node (id = 0) returns healthy by default in tests
	masterComps := mgr.NodeComponents(0)
	if len(masterComps) == 0 {
		t.Fatal("NodeComponents(0) returned no master components")
	}

	// Create a real node
	node, err := mgr.Store().CreateNode("comp-test-node", "198.51.100.20", "test-comp-tok")
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	now := time.Now().Unix()
	if err := mgr.Store().UpdateNodeStatus(node.ID, model.NodeStatusUpdate{LastSeen: now}); err != nil {
		t.Fatalf("UpdateNodeStatus: %v", err)
	}
	node.LastSeen = now

	// Set components for node
	comps := []nodeapi.ComponentStatus{
		{Name: nodeapi.ComponentXray, Status: nodeapi.StatusHealthy, Running: true},
		{Name: nodeapi.ComponentAWG, Status: nodeapi.StatusDegraded, Running: false, Error: "timeout"},
	}
	mgr.nodeGeoMu.Lock()
	mgr.nodeComponents[node.ID] = comps
	mgr.nodeGeoMu.Unlock()

	// Verify retrieval
	gotComps := mgr.NodeComponents(node.ID)
	if len(gotComps) != len(comps) {
		t.Fatalf("NodeComponents len = %d, want %d", len(gotComps), len(comps))
	}

	// Verify status is degraded (one healthy, one degraded)
	if st := mgr.NodeAggregatedStatus(node); st != "degraded" {
		t.Errorf("NodeAggregatedStatus() = %q, want degraded", st)
	}

	// If node becomes stale (offline)
	stale := time.Now().Unix() - 600 // 10 minutes ago
	if err := mgr.Store().UpdateNodeStatus(node.ID, model.NodeStatusUpdate{LastSeen: stale}); err != nil {
		t.Fatalf("UpdateNodeStatus: %v", err)
	}
	node.LastSeen = stale
	if st := mgr.NodeAggregatedStatus(node); st != "offline" {
		t.Errorf("NodeAggregatedStatus(stale) = %q, want offline", st)
	}
}
