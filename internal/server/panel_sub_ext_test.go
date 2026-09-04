package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/core"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func TestSubServersExternalAttachedWhenMasterFull(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	sup := xray.NewSupervisor("", filepath.Join(dir, "config.json"), dir)
	mgr := core.New(st, sup, xray.Options{PanelDest: "127.0.0.1:8080"}, core.TLSPaths{}, dir)
	h, err := New(mgr, "secretpath", "nginx", dir)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	rt := h.(*Router)

	// Create user
	u, err := st.CreateUser("client1", "uuid-1", "pw1", "token1", 0, 0, 0)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create node 1 (capacity plenty, not full)
	node, err := st.CreateNode("node1", "1.2.3.4", "default")
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	_ = st.UpdateNodeStatus(node.ID, model.NodeStatusUpdate{
		LastSeen:       time.Now().Unix(),
		XrayRunning:    true,
		CertSelfSigned: true,
		CertSHA256:     "mock-sha256",
	})

	// Master placement: capacity 1, HideWhenFull = true
	err = st.SetMasterPlacement(model.Placement{
		Capacity:     1,
		HideWhenFull: true,
	})
	if err != nil {
		t.Fatalf("set master placement: %v", err)
	}

	set, err := st.GetSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}

	// Add external subscription with enabled servers
	subID, err := st.CreateExtSubscription("https://example.com/sub", "test-sub")
	if err != nil {
		t.Fatalf("create extsub: %v", err)
	}
	_, _, _, err = st.ReplaceExtServers(subID, []model.ExtServer{
		{
			SubID:    subID,
			Key:      "ext1",
			Name:     "External Proxy 1",
			Protocol: "vless",
			Host:     "ext.example.com",
			Port:     443,
			Link:     "vless://ext1@ext.example.com:443#External",
			Enabled:  true,
		},
	}, 100)
	if err != nil {
		t.Fatalf("replace ext servers: %v", err)
	}

	// Simulate online connection on master so online count = 1 >= capacity 1
	mgr.RecordAccessOn(model.LocalNodeID, model.UserEmail(u.ID), "198.51.100.1", "")

	servers, err := rt.subServers(set, u.ID, "198.51.100.1")
	if err != nil {
		t.Fatalf("subServers: %v", err)
	}

	// Master should be hidden due to full capacity
	hasMaster := false
	for _, s := range servers {
		if s.Set.ServerID == model.LocalNodeID {
			hasMaster = true
		}
	}
	if hasMaster {
		t.Errorf("master server should be hidden when full, but was found in servers")
	}

	// But external servers MUST still be attached to the remaining servers
	hasExt := false
	for _, s := range servers {
		if len(s.External) > 0 {
			hasExt = true
			break
		}
	}
	if !hasExt {
		t.Fatalf("external servers were lost when master was hidden!")
	}
}
