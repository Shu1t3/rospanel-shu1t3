package mtproto

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/9seconds/mtg/v2/mtglib"
)

func TestMetricsAtomicCounters(t *testing.T) {
	m := NewMetrics()
	stream := NewEventStream(m)
	ctx := context.Background()

	// Simulate connection start
	stream.Send(ctx, mtglib.NewEventStart("s1", net.ParseIP("1.2.3.4")))
	if m.ActiveConns.Load() != 1 || m.TotalConns.Load() != 1 {
		t.Fatalf("expected 1 active & 1 total conn, got active=%d total=%d", m.ActiveConns.Load(), m.TotalConns.Load())
	}

	// Simulate traffic
	stream.Send(ctx, mtglib.NewEventTraffic("s1", 1024, true))  // read
	stream.Send(ctx, mtglib.NewEventTraffic("s1", 2048, false)) // write

	if m.BytesRead.Load() != 1024 {
		t.Fatalf("expected 1024 bytes read, got %d", m.BytesRead.Load())
	}
	if m.BytesWritten.Load() != 2048 {
		t.Fatalf("expected 2048 bytes written, got %d", m.BytesWritten.Load())
	}

	// Simulate concurrency limited
	stream.Send(ctx, mtglib.NewEventConcurrencyLimited())
	if m.ConcurrencyLimited.Load() != 1 || m.ErrorsCount.Load() != 1 {
		t.Fatalf("expected concurrency limited counter = 1, got %d", m.ConcurrencyLimited.Load())
	}

	// Simulate finish
	stream.Send(ctx, mtglib.NewEventFinish("s1"))
	if m.ActiveConns.Load() != 0 {
		t.Fatalf("expected 0 active conns, got %d", m.ActiveConns.Load())
	}
}

func TestSnapshotAndMemory(t *testing.T) {
	m := NewMetrics()
	snap := m.Snapshot()

	if snap.MemAlloc == 0 || snap.MemSys == 0 {
		t.Fatalf("expected non-zero memory stats: %+v", snap)
	}
	time.Sleep(10 * time.Millisecond)
	if m.Uptime() <= 0 {
		t.Fatal("expected positive uptime")
	}
}
