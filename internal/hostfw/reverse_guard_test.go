package hostfw

import (
	"strings"
	"testing"
)

func TestReverseGuardRulesetDualStack(t *testing.T) {
	policy := ReversePolicy{
		Transit: map[int][]string{29444: {"192.0.2.1", "2001:db8::1", "::ffff:192.0.2.1"}},
		BlockIP: []string{"198.51.100.2", "2001:db8::2"},
	}
	got, err := reverseRuleset(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"tcp dport 29444 ip saddr { 192.0.2.1 } return\n",
		"tcp dport 29444 ip6 saddr { 2001:db8::1 } return\n",
		"tcp dport 29444 drop\n",
		"ip daddr { 198.51.100.2 } drop\n",
		"ip6 daddr { 2001:db8::2 } drop\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if _, err := reverseRuleset(ReversePolicy{Transit: map[int][]string{29444: {}}}); err == nil {
		t.Fatal("empty relay allowlist accepted")
	}
	if _, err := reverseRuleset(ReversePolicy{BlockIP: []string{"::; drop"}}); err == nil {
		t.Fatal("non-IP input accepted")
	}
}
