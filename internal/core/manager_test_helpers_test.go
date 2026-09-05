package core

import (
	"path/filepath"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "core_test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	sup := xray.NewSupervisor("", filepath.Join(dir, "config.json"), dir)
	return New(st, sup, xray.Options{}, TLSPaths{}, dir)
}
