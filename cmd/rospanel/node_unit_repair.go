package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const nodeFirewallDropIn = "[Service]\nReadWritePaths=-/etc/ufw -/var/lib/ufw -/lib/ufw\n"

// Older node units kept /etc/ufw read-only. A binary-only self-update cannot
// replace the unit, so the first start of a new agent repairs the installed
// service and lets Restart=always start it again with the new mount namespace.
func repairNodeServiceFirewallAccess() (bool, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("INVOCATION_ID") == "" {
		return false, nil // manual run or a container without this systemd service
	}
	show := func() (string, error) {
		out, err := exec.Command("systemctl", "show", "rospanel-node.service", "-p", "ReadWritePaths", "--value").Output()
		return string(out), err
	}
	reload := func() error {
		out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl daemon-reload: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return ensureNodeServiceFirewallAccess(filepath.Join(filepath.Dir(nodeUnitPath), "rospanel-node.service.d", "rospanel-firewall.conf"), show, reload)
}

func ensureNodeServiceFirewallAccess(dropInPath string, show func() (string, error), reload func() error) (bool, error) {
	paths, err := show()
	if err != nil {
		return false, fmt.Errorf("read effective ReadWritePaths: %w", err)
	}
	if nodeUFWConfigWritable(paths) {
		return false, nil
	}
	if b, err := os.ReadFile(dropInPath); err == nil {
		if string(b) != nodeFirewallDropIn {
			return false, fmt.Errorf("existing systemd drop-in %s has custom content", dropInPath)
		}
		// A previous daemon-reload may have failed. Retry it, but verify the
		// effective property before restarting: a later administrator override
		// could otherwise leave the service in an endless restart loop.
		return reloadAndCheckNodeFirewallAccess(show, reload)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dropInPath), 0o755); err != nil {
		return false, fmt.Errorf("create systemd drop-in directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dropInPath), ".rospanel-firewall-*")
	if err != nil {
		return false, fmt.Errorf("create systemd drop-in: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(nodeFirewallDropIn); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Close(); err != nil {
		return false, err
	}
	if err = os.Rename(tmp.Name(), dropInPath); err != nil {
		return false, fmt.Errorf("install systemd drop-in: %w", err)
	}
	return reloadAndCheckNodeFirewallAccess(show, reload)
}

func reloadAndCheckNodeFirewallAccess(show func() (string, error), reload func() error) (bool, error) {
	if err := reload(); err != nil {
		return false, err
	}
	paths, err := show()
	if err != nil {
		return false, fmt.Errorf("read effective ReadWritePaths after daemon-reload: %w", err)
	}
	if !nodeUFWConfigWritable(paths) {
		return false, fmt.Errorf("drop-in loaded but effective ReadWritePaths still omits UFW directories")
	}
	return true, nil
}

func nodeUFWConfigWritable(raw string) bool {
	paths := map[string]bool{}
	for _, field := range strings.Fields(raw) {
		paths[filepath.Clean(strings.TrimPrefix(field, "-"))] = true
	}
	if paths["/"] {
		return true
	}
	return paths["/etc"] || paths["/etc/ufw"]
}
