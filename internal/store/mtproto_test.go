package store

import (
	"path/filepath"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
)

func TestMTProtoStoreCRUD(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "mtproto_crud.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	proxy := model.MTProtoProxy{
		Name:     "Test Standalone",
		Host:     "192.0.2.1",
		Port:     8443,
		Secret:   "ee112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		Domain:   "cloudflare.com",
		MaxConns: 256,
		Enabled:  true,
	}

	if err := st.CreateMTProtoProxy(&proxy); err != nil {
		t.Fatalf("CreateMTProtoProxy: %v", err)
	}
	if proxy.ID <= 0 {
		t.Fatalf("expected positive ID, got %d", proxy.ID)
	}
	if proxy.Token == "" {
		t.Fatalf("expected auto-generated token, got empty")
	}

	got, err := st.GetMTProtoProxy(proxy.ID)
	if err != nil {
		t.Fatalf("GetMTProtoProxy: %v", err)
	}
	if got == nil || got.Name != proxy.Name || got.Port != proxy.Port {
		t.Fatalf("unexpected got proxy: %+v", got)
	}

	byToken, err := st.GetMTProtoProxyByToken(proxy.Token)
	if err != nil {
		t.Fatalf("GetMTProtoProxyByToken: %v", err)
	}
	if byToken == nil || byToken.ID != proxy.ID {
		t.Fatalf("byToken mismatch: got %+v", byToken)
	}

	// Update
	proxy.Name = "Updated Standalone"
	proxy.Port = 9443
	if err := st.UpdateMTProtoProxy(proxy); err != nil {
		t.Fatalf("UpdateMTProtoProxy: %v", err)
	}

	gotUpdated, err := st.GetMTProtoProxy(proxy.ID)
	if err != nil {
		t.Fatalf("GetMTProtoProxy after update: %v", err)
	}
	if gotUpdated.Name != "Updated Standalone" || gotUpdated.Port != 9443 {
		t.Fatalf("updated proxy mismatch: %+v", gotUpdated)
	}

	// Heartbeat update
	snap := mtproto.Snapshot{
		Running:      true,
		ActiveConns:  42,
		BytesRead:    1024,
		BytesWritten: 2048,
		UptimeSec:    120,
		MemAlloc:     15 * 1024 * 1024,
		RSS:          25 * 1024 * 1024,
	}
	if err := st.UpdateMTProtoHeartbeat(proxy.Token, snap); err != nil {
		t.Fatalf("UpdateMTProtoHeartbeat: %v", err)
	}

	afterHB, err := st.GetMTProtoProxy(proxy.ID)
	if err != nil {
		t.Fatalf("GetMTProtoProxy after heartbeat: %v", err)
	}
	if !afterHB.Running || afterHB.ActiveConns != 42 || afterHB.BytesRead != 1024 || afterHB.BytesWritten != 2048 {
		t.Fatalf("heartbeat stats mismatch: %+v", afterHB)
	}

	// Toggle enabled
	if err := st.SetMTProtoProxyEnabled(proxy.ID, false); err != nil {
		t.Fatalf("SetMTProtoProxyEnabled: %v", err)
	}
	afterToggle, err := st.GetMTProtoProxy(proxy.ID)
	if err != nil {
		t.Fatalf("GetMTProtoProxy after toggle: %v", err)
	}
	if afterToggle.Enabled {
		t.Fatalf("expected proxy to be disabled")
	}

	// Delete
	if err := st.DeleteMTProtoProxy(proxy.ID); err != nil {
		t.Fatalf("DeleteMTProtoProxy: %v", err)
	}
	afterDel, err := st.GetMTProtoProxy(proxy.ID)
	if err != nil {
		t.Fatalf("GetMTProtoProxy after delete: %v", err)
	}
	if afterDel != nil {
		t.Fatalf("expected nil after delete, got %+v", afterDel)
	}
}

func TestMTProtoSettingsAndNode(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "mtproto_node.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	// 1. Master settings MTProto roundtrip
	masterCfg := model.MTProtoConfig{
		Enabled:  true,
		Port:     7443,
		Secret:   "ee00112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		Domain:   "cloudflare.com",
		MaxConns: 1024,
	}
	if err := st.SetMasterMTProto(masterCfg); err != nil {
		t.Fatalf("SetMasterMTProto: %v", err)
	}

	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !settings.MTProto.Enabled || settings.MTProto.Port != 7443 || settings.MTProto.MaxConns != 1024 {
		t.Fatalf("settings.MTProto mismatch: %+v", settings.MTProto)
	}

	// 2. Node mixed-mode MTProto roundtrip
	node, err := st.CreateNode("test-node", "node.example.com", "default")
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	nodeCfg := model.MTProtoConfig{
		Enabled:  true,
		Port:     8443,
		Secret:   "ee112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		Domain:   "cloudflare.com",
		MaxConns: 512,
	}
	if err := st.SetNodeMTProto(node.ID, nodeCfg); err != nil {
		t.Fatalf("SetNodeMTProto: %v", err)
	}

	nodeGot, err := st.GetNode(node.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if !nodeGot.MTProto.Enabled || nodeGot.MTProto.Port != 8443 {
		t.Fatalf("nodeGot.MTProto mismatch: %+v", nodeGot.MTProto)
	}

	// 3. ListAllMTProtoProxies includes standalone and node proxy
	standalone := model.MTProtoProxy{
		Name:    "Standalone",
		Host:    "standalone.example.com",
		Port:    9443,
		Secret:  "eeaabbccddeeff001122334455667788636c6f7564666c6172652e636f6d",
		Enabled: true,
	}
	if err := st.CreateMTProtoProxy(&standalone); err != nil {
		t.Fatalf("CreateMTProtoProxy: %v", err)
	}

	all, err := st.ListAllMTProtoProxies()
	if err != nil {
		t.Fatalf("ListAllMTProtoProxies: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 proxies (1 standalone + 1 node), got %d", len(all))
	}
}
