package xray

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeXray writes a shell script that mimics the xray CLI we drive: `run -test`
// exits 0 (config validation), `run -c <cfg>` blocks (a running daemon).
func fakeXray(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = run ] && [ \"$2\" = -test ]; then exit 0; fi\n" +
		"if [ \"$1\" = run ]; then exec sleep 60; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func newTestSup(t *testing.T) *Supervisor {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.json")
	return NewSupervisor(fakeXray(t), cfg, "")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (s *Supervisor) curPID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil || s.cur.cmd.Process == nil {
		return 0
	}
	return s.cur.cmd.Process.Pid
}

// TestAutoRestart: an unexpected exit (simulated crash) is auto-restarted.
func TestAutoRestart(t *testing.T) {
	s := newTestSup(t)
	if err := s.Apply(&Config{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	waitFor(t, "initial start", s.Running)
	old := s.curPID()
	if old == 0 {
		t.Fatal("no pid after start")
	}

	// Simulate a crash: kill the OS process directly (not via Stop), so the
	// monitor should treat it as unexpected and restart.
	s.mu.Lock()
	p := s.cur
	s.mu.Unlock()
	_ = p.cmd.Process.Kill()

	waitFor(t, "auto-restart with new pid", func() bool {
		return s.Running() && s.curPID() != 0 && s.curPID() != old
	})
	s.Stop()
}

// TestStopNoRestart: an intentional Stop must not be auto-restarted, and
// Running() must report false afterwards.
func TestStopNoRestart(t *testing.T) {
	s := newTestSup(t)
	if err := s.Apply(&Config{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	waitFor(t, "start", s.Running)
	s.Stop()
	if s.Running() {
		t.Fatal("still running right after Stop")
	}
	// Stays down (no resurrection by a stray supervise goroutine).
	time.Sleep(1500 * time.Millisecond)
	if s.Running() {
		t.Fatal("resurrected after Stop")
	}
}

// TestConcurrentApply: many concurrent Applies must serialize cleanly (run with
// -race) and leave exactly one process running.
func TestConcurrentApply(t *testing.T) {
	s := newTestSup(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Apply(&Config{}); err != nil {
				t.Errorf("apply: %v", err)
			}
		}()
	}
	wg.Wait()
	waitFor(t, "running after applies", s.Running)
	pid := s.curPID()
	time.Sleep(200 * time.Millisecond)
	if s.curPID() != pid {
		t.Fatalf("process churned after applies settled: %d -> %d", pid, s.curPID())
	}
	s.Stop()
}

func TestParseStats(t *testing.T) {
	data := []byte(`{
		"stat": [
			{"name": "user>>>u1>>>traffic>>>uplink", "value": "1048576"},
			{"name": "user>>>u1>>>traffic>>>downlink", "value": 2097152},
			{"name": "user>>>u2>>>traffic>>>uplink", "value": "512"},
			{"name": "inbound>>>api>>>traffic>>>downlink", "value": 100},
			{"name": "user>>>invalid", "value": 0},
			{"name": "user>>>u3>>>other>>>downlink", "value": 200},
			{"name": "user>>>u4>>>traffic>>>downlink>>>extra", "value": 300}
		]
	}`)
	res := parseStats(data)
	if res["u1"].Up != 1048576 || res["u1"].Down != 2097152 {
		t.Fatalf("unexpected u1 stats: %+v", res["u1"])
	}
	if res["u2"].Up != 512 || res["u2"].Down != 0 {
		t.Fatalf("unexpected u2 stats: %+v", res["u2"])
	}
	if _, ok := res["api"]; ok {
		t.Fatalf("inbound stat included")
	}
	if _, ok := res["invalid"]; ok {
		t.Fatalf("invalid stat included")
	}
	if _, ok := res["u3"]; ok {
		t.Fatalf("u3 non-traffic stat included")
	}
	if _, ok := res["u4"]; ok {
		t.Fatalf("u4 extra-part stat included")
	}
}

func BenchmarkParseStats(b *testing.B) {
	data := []byte(`{
		"stat": [
			{"name": "user>>>u1>>>traffic>>>uplink", "value": "1048576"},
			{"name": "user>>>u1>>>traffic>>>downlink", "value": 2097152},
			{"name": "user>>>u2>>>traffic>>>uplink", "value": "512"},
			{"name": "user>>>u2>>>traffic>>>downlink", "value": "1024"},
			{"name": "inbound>>>api>>>traffic>>>downlink", "value": 100},
			{"name": "user>>>invalid", "value": 0}
		]
	}`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = parseStats(data)
	}
}
