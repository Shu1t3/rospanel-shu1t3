package core

import (
	"testing"
)

func TestParseRejectIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "socks5 reject",
			line: "2026/11/17 20:00:00 [Warning] [12345678] from tcp:198.51.100.42:54321 rejected  proxy/socks: invalid username or password",
			want: "198.51.100.42",
		},
		{
			name: "http proxy reject",
			line: "2026/11/17 20:00:00 [Warning] [87654321] from tcp:203.0.113.99:43210 rejected  proxy/http: failed to parse client info",
			want: "203.0.113.99",
		},
		{
			name: "loopback ipv4 ignored",
			line: "2026/11/17 20:00:00 [Warning] [12345678] from tcp:127.0.0.1:54321 rejected  proxy/socks: invalid username or password",
			want: "",
		},
		{
			name: "loopback ipv6 ignored",
			line: "2026/11/17 20:00:00 [Warning] [12345678] from tcp:[::1]:54321 rejected  proxy/http: invalid auth",
			want: "",
		},
		{
			name: "accepted line ignored",
			line: "2026/11/17 20:00:00 [Info] [12345678] from tcp:198.51.100.42:54321 accepted tcp:example.com:443",
			want: "",
		},
		{
			name: "other protocol reject ignored",
			line: "2026/11/17 20:00:00 [Warning] [12345678] from tcp:198.51.100.42:54321 rejected  app/dispatcher: default route",
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseRejectIP(tc.line)
			if got != tc.want {
				t.Errorf("parseRejectIP(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestBruteGuardRecordThreshold(t *testing.T) {
	t.Parallel()
	g := newBruteGuard()
	ip := "198.51.100.5"

	for i := 1; i < bruteMaxTries; i++ {
		if g.record(ip) {
			t.Fatalf("attempt %d triggered ban early, threshold is %d", i, bruteMaxTries)
		}
	}
	if !g.record(ip) {
		t.Fatalf("attempt %d should trigger ban", bruteMaxTries)
	}
	// Once banned, further record calls should return false (already banned)
	if g.record(ip) {
		t.Fatal("subsequent attempt while banned should return false")
	}
}
