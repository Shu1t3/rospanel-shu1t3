package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureNodeServiceFirewallAccessRepairsOldUnitOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel-node.service.d", "rospanel-firewall.conf")
	paths := "/usr/local/bin /etc/systemd/system\n"
	show := func() (string, error) { return paths, nil }
	reloads := 0
	reload := func() error {
		reloads++
		if reloads == 1 {
			paths = "/usr/local/bin /etc/systemd/system -/etc/ufw -/var/lib/ufw -/lib/ufw\n"
		}
		return nil
	}

	restart, err := ensureNodeServiceFirewallAccess(path, show, reload)
	if err != nil || !restart || reloads != 1 {
		t.Fatalf("first start: restart=%v reloads=%d err=%v", restart, reloads, err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != nodeFirewallDropIn {
		t.Fatalf("drop-in: %q, %v", b, err)
	}

	// If a later administrator override defeats the drop-in, keep serving instead
	// of repeatedly restarting the node.
	paths = "/usr/local/bin /etc/systemd/system\n"
	restart, err = ensureNodeServiceFirewallAccess(path, show, reload)
	if restart || err == nil || reloads != 2 {
		t.Fatalf("second start: restart=%v reloads=%d err=%v", restart, reloads, err)
	}
}

func TestEnsureNodeServiceFirewallAccessRetriesFailedReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel-firewall.conf")
	paths := "/usr/local/bin /etc/systemd/system\n"
	show := func() (string, error) { return paths, nil }
	reloads := 0
	reload := func() error {
		reloads++
		if reloads == 1 {
			return errors.New("temporary systemd failure")
		}
		paths = "/usr/local/bin /etc/systemd/system -/etc/ufw -/var/lib/ufw -/lib/ufw\n"
		return nil
	}
	if restart, err := ensureNodeServiceFirewallAccess(path, show, reload); restart || err == nil {
		t.Fatalf("failed reload: restart=%v err=%v", restart, err)
	}
	if restart, err := ensureNodeServiceFirewallAccess(path, show, reload); !restart || err != nil || reloads != 2 {
		t.Fatalf("retry reload: restart=%v reloads=%d err=%v", restart, reloads, err)
	}
}

func TestEnsureNodeServiceFirewallAccessSkipsFixedUnit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel-firewall.conf")
	show := func() (string, error) {
		return "/usr/local/bin /etc/systemd/system -/etc/ufw -/var/lib/ufw -/lib/ufw\n", nil
	}
	restart, err := ensureNodeServiceFirewallAccess(path, show, func() error { t.Fatal("unexpected daemon-reload"); return nil })
	if err != nil || restart {
		t.Fatalf("fixed unit: restart=%v err=%v", restart, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("fixed unit wrote a drop-in: %v", err)
	}
}

func TestEnsureNodeServiceFirewallAccessKeepsServingOnSystemdFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel-firewall.conf")
	restart, err := ensureNodeServiceFirewallAccess(path, func() (string, error) {
		return "", errors.New("systemd unavailable")
	}, func() error { t.Fatal("unexpected daemon-reload"); return nil })
	if restart || err == nil {
		t.Fatalf("systemd failure: restart=%v err=%v", restart, err)
	}
}
