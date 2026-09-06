package mtproto

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestConcurrentConnectionsAndMemoryBudget(t *testing.T) {
	sec, err := GenerateSecret("cloudflare.com")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1:0"
	cfg.Port = 0
	cfg.Secret = sec
	cfg.MaxConns = 512

	lifecycle, err := NewLifecycle(cfg)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = lifecycle.Run(ctx)
	}()

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

	memBefore := ReadMemoryStats()
	t.Logf("Memory before load: Alloc=%d KB, HeapInuse=%d KB, RSS=%d KB",
		memBefore.Alloc/1024, memBefore.HeapInuse/1024, memBefore.RSS/1024)

	// Simulate concurrent connections
	const concurrentClients = 80
	var wg sync.WaitGroup
	wg.Add(concurrentClients)

	for i := 0; i < concurrentClients; i++ {
		go func() {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()
			// Send dummy bytes to trigger handshake handling
			_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: cloudflare.com\r\n\r\n"))
			time.Sleep(50 * time.Millisecond)
		}()
	}

	wg.Wait()

	memDuring := ReadMemoryStats()
	t.Logf("Memory after concurrency load: Alloc=%d KB, HeapInuse=%d KB, RSS=%d KB",
		memDuring.Alloc/1024, memDuring.HeapInuse/1024, memDuring.RSS/1024)

	// Verify that heap alloc is well below the 96 MB container boundary (< 32 MB)
	const maxAllowedHeapBytes = 32 * 1024 * 1024
	if memDuring.Alloc > maxAllowedHeapBytes {
		t.Errorf("Heap allocation too high: %d bytes (limit: %d bytes)", memDuring.Alloc, maxAllowedHeapBytes)
	}

	// If RSS is available, check that it's comfortably below 96 MB (< 80 MB)
	const maxAllowedRSSBytes = 80 * 1024 * 1024
	if memDuring.RSS > 0 && memDuring.RSS > maxAllowedRSSBytes {
		t.Errorf("Process RSS exceeded safe headroom: %d bytes (limit: %d bytes)", memDuring.RSS, maxAllowedRSSBytes)
	}
}
