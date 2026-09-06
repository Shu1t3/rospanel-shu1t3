package mtproto

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/9seconds/mtg/v2/mtglib"
)

// Metrics tracks runtime performance and usage counters for the MTProto proxy.
// All fields use Go atomic primitives to ensure thread-safety with zero lock contention.
type Metrics struct {
	ActiveConns        atomic.Int64
	TotalConns         atomic.Uint64
	BytesRead          atomic.Uint64
	BytesWritten       atomic.Uint64
	ErrorsCount        atomic.Uint64
	ConcurrencyLimited atomic.Uint64
	startedAt          time.Time
}

// NewMetrics initializes a new Metrics tracker.
func NewMetrics() *Metrics {
	return &Metrics{
		startedAt: time.Now(),
	}
}

// Uptime returns the duration since proxy start.
func (m *Metrics) Uptime() time.Duration {
	if m.startedAt.IsZero() {
		return 0
	}
	return time.Since(m.startedAt)
}

// Snapshot returns a point-in-time copy of metrics.
type Snapshot struct {
	Running            bool   `json:"running"`
	ActiveConns        int64  `json:"active_conns"`
	TotalConns         uint64 `json:"total_conns"`
	BytesRead          uint64 `json:"bytes_read"`
	BytesWritten       uint64 `json:"bytes_written"`
	ErrorsCount        uint64 `json:"errors_count"`
	ConcurrencyLimited uint64 `json:"concurrency_limited"`
	UptimeSec          int64  `json:"uptime_sec"`
	MemAlloc           uint64 `json:"mem_alloc"`
	MemSys             uint64 `json:"mem_sys"`
	MemHeapInuse       uint64 `json:"mem_heap_inuse"`
	RSS                uint64 `json:"rss"`
	NumGC              uint32 `json:"num_gc"`
	LastError          string `json:"last_error,omitempty"`
}

// Snapshot captures the current counters and memory statistics.
func (m *Metrics) Snapshot() Snapshot {
	mem := ReadMemoryStats()
	return Snapshot{
		ActiveConns:        m.ActiveConns.Load(),
		TotalConns:         m.TotalConns.Load(),
		BytesRead:          m.BytesRead.Load(),
		BytesWritten:       m.BytesWritten.Load(),
		ErrorsCount:        m.ErrorsCount.Load(),
		ConcurrencyLimited: m.ConcurrencyLimited.Load(),
		UptimeSec:          int64(m.Uptime().Seconds()),
		MemAlloc:           mem.Alloc,
		MemSys:             mem.Sys,
		MemHeapInuse:       mem.HeapInuse,
		RSS:                mem.RSS,
		NumGC:              mem.NumGC,
	}
}

// MemStatsInfo encapsulates key memory usage indicators.
type MemStatsInfo struct {
	Alloc     uint64 `json:"alloc"`
	Sys       uint64 `json:"sys"`
	HeapInuse uint64 `json:"heap_inuse"`
	RSS       uint64 `json:"rss"`
	NumGC     uint32 `json:"num_gc"`
}

// ReadMemoryStats retrieves current Go heap and process RSS memory metrics.
func ReadMemoryStats() MemStatsInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return MemStatsInfo{
		Alloc:     m.Alloc,
		Sys:       m.Sys,
		HeapInuse: m.HeapInuse,
		RSS:       readProcessRSS(),
		NumGC:     m.NumGC,
	}
}

// readProcessRSS attempts to read the process Resident Set Size in bytes.
func readProcessRSS() uint64 {
	// Linux /proc/self/statm
	if data, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 2 {
			if pages, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				return pages * uint64(syscall.Getpagesize())
			}
		}
	}

	// Fallback via getrusage
	var r syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err == nil {
		if runtime.GOOS == "darwin" {
			// On Darwin, Maxrss is reported in bytes
			return uint64(r.Maxrss)
		}
		// On Linux, Maxrss is reported in kilobytes
		return uint64(r.Maxrss) * 1024
	}
	return 0
}

// atomicEventStream is a zero-alloc implementation of mtglib.EventStream that
// updates atomic metrics directly without queueing or background workers.
type atomicEventStream struct {
	metrics *Metrics
}

// NewEventStream returns an mtglib.EventStream backed by the given Metrics.
func NewEventStream(m *Metrics) mtglib.EventStream {
	return &atomicEventStream{metrics: m}
}

func (s *atomicEventStream) Send(_ context.Context, evt mtglib.Event) {
	if s.metrics == nil {
		return
	}
	switch e := evt.(type) {
	case mtglib.EventStart:
		s.metrics.ActiveConns.Add(1)
		s.metrics.TotalConns.Add(1)
	case mtglib.EventFinish:
		s.metrics.ActiveConns.Add(-1)
	case mtglib.EventTraffic:
		if e.IsRead {
			s.metrics.BytesRead.Add(uint64(e.Traffic))
		} else {
			s.metrics.BytesWritten.Add(uint64(e.Traffic))
		}
	case mtglib.EventConcurrencyLimited:
		s.metrics.ConcurrencyLimited.Add(1)
		s.metrics.ErrorsCount.Add(1)
	}
}
