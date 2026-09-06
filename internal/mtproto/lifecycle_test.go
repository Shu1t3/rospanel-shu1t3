package mtproto

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestLifecycleRunAndAtomicReload(t *testing.T) {
	sec1, err := GenerateSecret("cloudflare.com")
	if err != nil {
		t.Fatalf("generate secret 1: %v", err)
	}

	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1:0"
	cfg.Port = 0
	cfg.Secret = sec1
	cfg.MaxConns = 100

	lifecycle, err := NewLifecycle(cfg)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- lifecycle.Run(ctx)
	}()

	// Wait until listening
	var addr string
	for i := 0; i < 50; i++ {
		lifecycle.mu.Lock()
		if lifecycle.realListener != nil {
			addr = lifecycle.realListener.Addr().String()
			lifecycle.mu.Unlock()
			break
		}
		lifecycle.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}

	if addr == "" {
		t.Fatal("proxy failed to start listening in time")
	}

	select {
	case err := <-runErr:
		t.Fatalf("lifecycle.Run exited early with err: %v", err)
	default:
	}

	// 1. Verify TCP connectivity to proxy
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial proxy listener at %s: %v", addr, err)
	}
	_ = conn.Close()

	// 2. Perform Atomic Reload with new secret
	sec2, err := GenerateSecret("google.com")
	if err != nil {
		t.Fatalf("generate secret 2: %v", err)
	}

	newCfg := cfg
	newCfg.Secret = sec2
	newCfg.MaxConns = 200

	if err := lifecycle.Reload(newCfg); err != nil {
		t.Fatalf("atomic reload failed: %v", err)
	}

	if lifecycle.CurrentConfig().Secret != sec2 {
		t.Fatalf("expected updated secret %s, got %s", sec2, lifecycle.CurrentConfig().Secret)
	}

	// 3. Verify connectivity persists after atomic reload
	conn2, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial proxy listener after reload at %s: %v", addr, err)
	}
	_ = conn2.Close()

	// 4. Cancel context and verify clean graceful shutdown
	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("unexpected error on shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for proxy shutdown")
	}
}
