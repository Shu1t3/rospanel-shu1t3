package core

import (
	"errors"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestReservedPortsNodeReleasesDisabledPorts(t *testing.T) {
	t.Parallel()

	nodeSet := &model.Settings{
		ServerID:        10,
		VLESSPort:       443,
		RealityPort:     443,
		HysteriaPort:    443,
		VLESSEnabled:    false,
		RealityEnabled:  false,
		HysteriaEnabled: true,
	}

	r := reservedPorts(nodeSet)

	if who, taken := r.OnTCP(443); taken {
		t.Fatalf("expected TCP/443 to be free on remote node when VLESS and REALITY are disabled, got held by: %s", who)
	}

	// UDP 443 should still be held by Hysteria2
	if who, taken := r.OnUDP(443); !taken || who != "HYSTERIA-UDP" {
		t.Fatalf("expected UDP/443 to be held by HYSTERIA-UDP, got held=%v, who=%s", taken, who)
	}
}

func TestReservedPortsMasterRetainsVLESSWhenDisabled(t *testing.T) {
	t.Parallel()

	masterSet := &model.Settings{
		ServerID:        model.LocalNodeID,
		VLESSPort:       443,
		RealityPort:     8443,
		HysteriaPort:    443,
		VLESSEnabled:    false,
		RealityEnabled:  false,
		HysteriaEnabled: true,
	}

	r := reservedPorts(masterSet)

	// On master, VLESSPort must remain held for panel fallback
	if who, taken := r.OnTCP(443); !taken || who != "VLESS-Vision" {
		t.Fatalf("expected TCP/443 on master to be held by VLESS-Vision for panel fallback, got held=%v, who=%s", taken, who)
	}

	// Reality is disabled and not on master fallback, so 8443 should be free
	if who, taken := r.OnTCP(8443); taken {
		t.Fatalf("expected TCP/8443 on master to be free when REALITY is disabled, got held by: %s", who)
	}
}

func TestReservedPortsNodeHoldsEnabledPorts(t *testing.T) {
	t.Parallel()

	nodeSet := &model.Settings{
		ServerID:       10,
		VLESSPort:      443,
		RealityPort:    8443,
		VLESSEnabled:   true,
		RealityEnabled: true,
	}

	r := reservedPorts(nodeSet)

	if who, taken := r.OnTCP(443); !taken || who != "VLESS-Vision" {
		t.Fatalf("expected TCP/443 to be held by VLESS-Vision, got held=%v, who=%s", taken, who)
	}
	if who, taken := r.OnTCP(8443); !taken || who != "VLESS-XHTTP-REALITY" {
		t.Fatalf("expected TCP/8443 to be held by VLESS-XHTTP-REALITY, got held=%v, who=%s", taken, who)
	}
}

func TestApplyNodeConnectionsRejectsCollisionWithCustomInbound(t *testing.T) {
	t.Parallel()
	m := nodeTestManager(t)

	node, err := m.store.CreateNode("node-collision-test", "test.example.com", "")
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	// Create custom inbound on port 443 TCP
	custom := model.Inbound{
		ServerID: node.ID,
		Name:     "Custom-TCP-443",
		Protocol: model.InbVLESS,
		Port:     443,
		Enabled:  true,
		Opts: model.InboundOpts{
			Transport: "tcp",
			Security:  model.SecReality,
		},
	}
	custom.Normalize()
	if _, err := m.store.CreateInbound(custom); err != nil {
		t.Fatalf("create custom inbound: %v", err)
	}

	checkCode := func(err error, want string) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected error code %s, got nil", want)
		}
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Code != want {
			t.Fatalf("expected code %s, got err: %v", want, err)
		}
	}

	// 1. Enabling VLESS (which defaults to port 443) should fail due to collision with Custom-TCP-443
	err = m.ApplyNodeConnections(node.ID, ConnectionsUpdate{
		Protocols:    map[string]bool{"vless": true, "hysteria2": false, "reality": false},
		HopStart:    20000,
		HopEnd:      30000,
		HysteriaPort: 8443,
		RealityPort:  8444,
	})
	checkCode(err, "err.portTakenByInbound")

	// 2. Enabling REALITY with RealityPort = 443 should also fail
	err = m.ApplyNodeConnections(node.ID, ConnectionsUpdate{
		Protocols:    map[string]bool{"vless": false, "hysteria2": false, "reality": true},
		HopStart:    20000,
		HopEnd:      30000,
		HysteriaPort: 8443,
		RealityPort:  443,
	})
	checkCode(err, "err.portTakenByInbound")

	// 3. Enabling REALITY on a non-conflicting port (e.g. 8444) should succeed
	err = m.ApplyNodeConnections(node.ID, ConnectionsUpdate{
		Protocols:    map[string]bool{"vless": false, "hysteria2": false, "reality": true},
		HopStart:    20000,
		HopEnd:      30000,
		HysteriaPort: 8443,
		RealityPort:  8444,
	})
	if err != nil {
		t.Fatalf("expected ApplyNodeConnections to succeed for non-conflicting port, got %v", err)
	}
}
