package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
	"github.com/Shu1t3/rospanel-shu1t3/internal/nodeapi"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func TestManagerListAllMTProtoProxies(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "mtproto_mgr.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	mgr := &Manager{
		store:            st,
		nodes:            newNodeRegistry(),
		opts:             xray.Options{PanelDest: "127.0.0.1:8080"},
		tz:               time.Local,
		applied:          map[int64]struct{}{},
		nodeGeoFiles:     map[int64][]nodeapi.GeoFile{},
		nodeHostStats:    map[int64]nodeapi.HostStats{},
		nodeSyncFails:    map[int64]int{},
		nodeAWGRunning:   map[int64]bool{},
		nodeAWGErr:       map[int64]string{},
		nodeComponents:   map[int64][]nodeapi.ComponentStatus{},
		nodeMTProtoStats: map[int64]mtproto.Snapshot{},
	}

	// 1. Enable Master MTProto
	masterCfg := model.MTProtoConfig{
		Enabled:  true,
		Port:     8443,
		Secret:   "ee00112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		Domain:   "cloudflare.com",
		MaxConns: 512,
	}
	if err := st.SetMasterMTProto(masterCfg); err != nil {
		t.Fatalf("SetMasterMTProto: %v", err)
	}

	// 2. Create Node with MTProto
	node, err := st.CreateNode("node-fi", "node-fi.example.com", "default")
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	nodeCfg := model.MTProtoConfig{
		Enabled:  true,
		Port:     8443,
		Secret:   "ee00112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		Domain:   "yle.fi",
		MaxConns: 200,
	}
	if err := st.SetNodeMTProto(node.ID, nodeCfg); err != nil {
		t.Fatalf("SetNodeMTProto: %v", err)
	}

	// Simulate node reporting MTProto live stats
	_, err = mgr.IngestNodeSync(node, nodeapi.SyncRequest{
		ConfigHash: "h1",
		MTProtoSnapshot: &mtproto.Snapshot{
			Running:      true,
			ActiveConns:  12,
			BytesRead:    1024,
			BytesWritten: 2048,
			UptimeSec:    360,
			RSS:          15 * 1024 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("IngestNodeSync: %v", err)
	}

	// 3. Create Standalone proxy
	standalone := model.MTProtoProxy{
		Name:    "Standalone Proxy",
		Host:    "stand.example.com",
		Port:    9443,
		Secret:  "ee112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		Enabled: true,
	}
	if err := st.CreateMTProtoProxy(&standalone); err != nil {
		t.Fatalf("CreateMTProtoProxy: %v", err)
	}

	// 4. Query all proxies through Manager
	proxies, err := mgr.ListAllMTProtoProxies()
	if err != nil {
		t.Fatalf("ListAllMTProtoProxies: %v", err)
	}
	if len(proxies) != 3 {
		t.Fatalf("expected 3 proxies, got %d", len(proxies))
	}

	var foundMaster, foundNode, foundStandalone bool
	for _, p := range proxies {
		if p.ID == 0 {
			foundMaster = true
			if p.Port != 8443 {
				t.Fatalf("expected master port 8443, got %d", p.Port)
			}
		} else if p.ID < 0 {
			foundNode = true
			if p.Port != 8443 || p.Domain != "yle.fi" {
				t.Fatalf("expected node domain yle.fi, got %s", p.Domain)
			}
			if !p.Running || p.ActiveConns != 12 || p.UptimeSec != 360 {
				t.Fatalf("node proxy live stats not enriched: %+v", p)
			}
		} else {
			foundStandalone = true
			if p.Port != 9443 {
				t.Fatalf("expected standalone port 9443, got %d", p.Port)
			}
		}
	}

	if !foundMaster || !foundNode || !foundStandalone {
		t.Fatalf("missing proxies: master=%v, node=%v, standalone=%v", foundMaster, foundNode, foundStandalone)
	}
}
