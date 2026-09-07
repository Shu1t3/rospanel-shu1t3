package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func newCloseTestManager(t *testing.T) (*Manager, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "close.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	sup := xray.NewSupervisor("", filepath.Join(dir, "config.json"), dir)
	return New(st, sup, xray.Options{}, TLSPaths{}, dir), st
}

func TestCloseStopsManagerLoopsBeforeStoreClose(t *testing.T) {
	m, st := newCloseTestManager(t)
	finished := make(chan struct{})
	go func() {
		m.Close()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Close did not stop manager loops promptly")
	}
	if err := st.Close(); err != nil {
		t.Errorf("close store: %v", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	m, st := newCloseTestManager(t)
	defer st.Close()
	m.Close()
	m.Close()
}
