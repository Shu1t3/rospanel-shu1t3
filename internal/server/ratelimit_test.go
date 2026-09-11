package server

import (
	"fmt"
	"testing"
	"time"
)

// A locked-out IP must stay locked out even when an attacker floods the limiter
// with thousands of throwaway addresses. The sweep used to clear the whole IP map
// once it passed maxKeys, which handed the banned attacker a fresh attempt budget
// — spraying unique IPs was a way to un-ban yourself.
func TestLoginLimiterFloodDoesNotClearLockout(t *testing.T) {
	l := newLoginLimiter()

	const victim = "203.0.113.7"
	for i := 0; i < l.maxFails; i++ {
		l.fail(victim, "admin")
	}
	if !l.blocked(victim, "admin") {
		t.Fatal("attacker IP not locked out after maxFails")
	}

	// Flood well past maxKeys with unique, non-blocked addresses to force a sweep.
	for i := 0; i < l.maxKeys*2; i++ {
		l.fail(fmt.Sprintf("198.51.100.%d.%d", i/256, i%256), "")
	}

	if !l.blocked(victim, "admin") {
		t.Fatal("lockout was cleared by an unrelated IP flood — attacker regained a fresh budget")
	}
	if len(l.ips) > l.maxKeys {
		t.Fatalf("IP map unbounded after flood: %d entries (cap %d)", len(l.ips), l.maxKeys)
	}
}

// Even if every tracked IP is locked out, memory must stay bounded: the sweep
// evicts the lockouts closest to expiring rather than growing without limit.
func TestLoginLimiterBoundedWhenAllBlocked(t *testing.T) {
	l := newLoginLimiter()
	for i := 0; i < l.maxKeys+500; i++ {
		ip := fmt.Sprintf("198.51.100.%d.%d", i/256, i%256)
		for j := 0; j < l.maxFails; j++ {
			l.fail(ip, "")
		}
	}
	if len(l.ips) > l.maxKeys {
		t.Fatalf("IP map grew past cap with all-blocked entries: %d (cap %d)", len(l.ips), l.maxKeys)
	}
}

func TestIPRateLimiterFloodDoesNotClearRateLimit(t *testing.T) {
	l := newIPRateLimiter(5, 5*time.Minute)
	const victim = "203.0.113.7"
	for i := 0; i < 5; i++ {
		if !l.allow(victim) {
			t.Fatalf("unexpected rate limit hit early on attempt %d", i)
		}
	}
	if l.allow(victim) {
		t.Fatal("victim IP should be rate-limited after 5 hits")
	}

	// Flood well past maxKeys with unique addresses making 1 hit each
	for i := 0; i < l.maxKeys*2; i++ {
		l.allow(fmt.Sprintf("198.51.100.%d.%d", i/256, i%256))
	}

	if l.allow(victim) {
		t.Fatal("rate-limited victim IP was unblocked by an unrelated IP flood")
	}
	if len(l.hits) > l.maxKeys {
		t.Fatalf("hits map grew unbounded after flood: %d (cap %d)", len(l.hits), l.maxKeys)
	}
}

