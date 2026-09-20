package xray

import (
	"strings"
	"testing"
)

func TestVersionMatchesPinned(t *testing.T) {
	// The reported version (from `xray version`) has no "v"; PinnedVersion does.
	clean := strings.TrimPrefix(PinnedVersion, "v")
	for _, v := range []string{clean, "v" + clean} {
		if !VersionMatchesPinned(v) {
			t.Errorf("VersionMatchesPinned(%q) = false, want true (pinned=%s)", v, PinnedVersion)
		}
	}
	if VersionMatchesPinned("26.6.26") {
		t.Error("a genuinely different version must not match")
	}
}
