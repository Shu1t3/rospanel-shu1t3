package hostfw

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

const reverseTable = "rospanel_reverse_chain"

// ReversePolicy is the mandatory host filter for protected multi-hop routes.
// Transit ports accept only the listed relay source addresses. Local roles
// cannot send to, or accept connections from, any listed transit address.
type ReversePolicy struct {
	Transit map[int][]string `json:"transit,omitempty"`
	BlockIP []string         `json:"block_ip,omitempty"`
}

func (p ReversePolicy) empty() bool { return len(p.Transit) == 0 && len(p.BlockIP) == 0 }

func combineReversePolicy(a, b ReversePolicy) ReversePolicy {
	p := ReversePolicy{Transit: make(map[int][]string), BlockIP: append(slices.Clone(a.BlockIP), b.BlockIP...)}
	for _, source := range []ReversePolicy{a, b} {
		for port, ips := range source.Transit {
			p.Transit[port] = append(p.Transit[port], ips...)
		}
	}
	slices.Sort(p.BlockIP)
	p.BlockIP = slices.Compact(p.BlockIP)
	for port, ips := range p.Transit {
		slices.Sort(ips)
		p.Transit[port] = slices.Compact(ips)
	}
	return p
}

// ReverseGuard retains the rules protecting the old config during an Xray
// apply, including when the supervisor rolls back after a failure.
type ReverseGuard struct {
	mu          sync.Mutex
	current     ReversePolicy
	initialized bool
}

// Keep repairs rules removed by an out-of-band firewall reload. An external
// ingress ACL on B is still needed to cover the interval after such a removal.
func (g *ReverseGuard) Keep(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	var lastError string
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := g.refresh(ctx)
			if err == nil {
				lastError = ""
			} else if msg := err.Error(); msg != lastError && ctx.Err() == nil {
				slog.Error("protected route firewall: could not refresh policy", "err", msg)
				lastError = msg
			}
		}
	}
}

func (g *ReverseGuard) refresh(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current.empty() {
		return nil
	}
	return installReversePolicy(ctx, g.current)
}

func (g *ReverseGuard) Apply(policy *ReversePolicy, apply func() error) error {
	var next ReversePolicy
	if policy != nil {
		next = *policy
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current.empty() && next.empty() {
		if g.initialized {
			return apply()
		}
		// After a restart there can be a table left by an older process.
		// Remove it only after Xray has applied the unprotected config.
		if !reverseTableExists() {
			g.initialized = true
			return apply()
		}
		if err := apply(); err != nil {
			return err
		}
		if err := installReversePolicy(context.Background(), next); err != nil {
			return err
		}
		g.initialized = true
		return nil
	}
	combined := combineReversePolicy(g.current, next)
	if err := installReversePolicy(context.Background(), combined); err != nil {
		return err
	}
	if err := apply(); err != nil {
		// Keep the stricter local blocks and the inbound allowlists covering
		// both configurations if the runtime couldn't settle on either.
		g.current = combined
		return err
	}
	if err := installReversePolicy(context.Background(), next); err != nil {
		g.current = combined
		return err
	}
	g.current = next
	g.initialized = true
	return nil
}

func reverseTableExists() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "nft", "list", "table", "inet", reverseTable).Run() == nil
}

func installReversePolicy(ctx context.Context, p ReversePolicy) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rules, err := reverseRuleset(p)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(rules)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install protected route firewall: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func reverseRuleset(p ReversePolicy) (string, error) {
	var script strings.Builder
	// add is idempotent and makes the delete valid on the first apply. nft
	// executes the complete batch atomically, so replacing the table has no gap.
	fmt.Fprintf(&script, "add table inet %[1]s\ndelete table inet %[1]s\n", reverseTable)
	if p.empty() {
		return script.String(), nil
	}
	fmt.Fprintf(&script, "table inet %s {\n chain input { type filter hook input priority -10; policy accept;\n", reverseTable)
	block, err := reverseIPGroups(p.BlockIP)
	if err != nil {
		return "", err
	}
	for i, family := range []string{"ip", "ip6"} {
		if len(block[i]) > 0 {
			fmt.Fprintf(&script, "  %s saddr { %s } drop\n", family, strings.Join(block[i], ", "))
		}
	}
	var ports []int
	for port := range p.Transit {
		ports = append(ports, port)
	}
	slices.Sort(ports)
	for _, port := range ports {
		if port < 1 || port > 65535 || len(p.Transit[port]) == 0 {
			return "", fmt.Errorf("invalid protected transit port or relay allowlist: %d", port)
		}
		sources, err := reverseIPGroups(p.Transit[port])
		if err != nil {
			return "", err
		}
		for i, family := range []string{"ip", "ip6"} {
			if len(sources[i]) > 0 {
				fmt.Fprintf(&script, "  tcp dport %d %s saddr { %s } return\n", port, family, strings.Join(sources[i], ", "))
			}
		}
		fmt.Fprintf(&script, "  tcp dport %d drop\n", port)
	}
	script.WriteString(" }\n chain output { type filter hook output priority -10; policy accept;\n")
	for i, family := range []string{"ip", "ip6"} {
		if len(block[i]) > 0 {
			fmt.Fprintf(&script, "  %s daddr { %s } drop\n", family, strings.Join(block[i], ", "))
		}
	}
	script.WriteString(" }\n}\n")
	return script.String(), nil
}

func reverseIPGroups(ips []string) ([2][]string, error) {
	var groups [2][]string
	for _, raw := range ips {
		addr, err := netip.ParseAddr(raw)
		if err != nil || addr.Zone() != "" || addr.IsUnspecified() || addr.IsMulticast() {
			return groups, fmt.Errorf("invalid protected route IP %q", raw)
		}
		addr = addr.Unmap()
		i := 0
		if addr.Is6() {
			i = 1
		}
		groups[i] = append(groups[i], addr.String())
	}
	for i := range groups {
		slices.Sort(groups[i])
		groups[i] = slices.Compact(groups[i])
	}
	return groups, nil
}
