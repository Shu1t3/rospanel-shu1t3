package xray

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/Shu1t3/rospanel-shu1t3/internal/xray/statsrpc"
)

func TestParseProtoStats(t *testing.T) {
	stats := []*statsrpc.Stat{
		{Name: "user>>>u1>>>traffic>>>uplink", Value: 1048576},
		{Name: "user>>>u1>>>traffic>>>downlink", Value: 2097152},
		{Name: "user>>>u2>>>traffic>>>uplink", Value: 512},
		{Name: "inbound>>>api>>>traffic>>>downlink", Value: 100},
		{Name: "user>>>invalid", Value: 0},
		{Name: "user>>>u3>>>other>>>downlink", Value: 200},
		{Name: "user>>>u4>>>traffic>>>downlink>>>extra", Value: 300},
		nil, // ensure safe against nil entry
	}

	res := parseProtoStats(stats)
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

type mockStatsServer struct {
	statsrpc.UnimplementedStatsServiceServer
	mu       sync.Mutex
	queries  atomic.Int64
	userUp   int64
	userDown int64
}

func (m *mockStatsServer) QueryStats(ctx context.Context, req *statsrpc.QueryStatsRequest) (*statsrpc.QueryStatsResponse, error) {
	m.queries.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.Pattern == "inbound>>>" {
		return &statsrpc.QueryStatsResponse{
			Stat: []*statsrpc.Stat{
				{Name: "inbound>>>api>>>traffic>>>downlink", Value: 42},
			},
		}, nil
	}

	return &statsrpc.QueryStatsResponse{
		Stat: []*statsrpc.Stat{
			{Name: "user>>>u1>>>traffic>>>uplink", Value: m.userUp},
			{Name: "user>>>u1>>>traffic>>>downlink", Value: m.userDown},
			{Name: "user>>>u2>>>traffic>>>uplink", Value: 100},
			{Name: "inbound>>>api>>>traffic>>>downlink", Value: 50},
		},
	}, nil
}

func startMockGRPCServer(t *testing.T, srv *mockStatsServer) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	statsrpc.RegisterStatsServiceServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()

	return lis.Addr().String(), func() {
		s.Stop()
		_ = lis.Close()
	}
}

func TestSupervisorQueryStatsGRPC(t *testing.T) {
	mock := &mockStatsServer{userUp: 5000, userDown: 15000}
	addr, stop := startMockGRPCServer(t, mock)
	defer stop()

	sup := &Supervisor{}
	defer sup.closeGRPC()

	// Initial query
	res, err := sup.QueryStats(addr)
	if err != nil {
		t.Fatalf("QueryStats failed: %v", err)
	}
	if res["u1"].Up != 5000 || res["u1"].Down != 15000 {
		t.Fatalf("unexpected stats: %+v", res["u1"])
	}
	if res["u2"].Up != 100 || res["u2"].Down != 0 {
		t.Fatalf("unexpected u2 stats: %+v", res["u2"])
	}
	if mock.queries.Load() != 1 {
		t.Fatalf("expected 1 query to mock server, got %d", mock.queries.Load())
	}

	// Immediate second query within coalescing cache (1.5s) should return cached values
	mock.mu.Lock()
	mock.userUp = 99999
	mock.mu.Unlock()

	res2, err := sup.QueryStats(addr)
	if err != nil {
		t.Fatalf("QueryStats cached failed: %v", err)
	}
	if res2["u1"].Up != 5000 {
		t.Fatalf("expected cached userUp 5000, got %d", res2["u1"].Up)
	}
	if mock.queries.Load() != 1 {
		t.Fatalf("expected mock queries to still be 1 due to cache, got %d", mock.queries.Load())
	}

	// After cache expires, fresh stats are fetched
	sup.statsMu.Lock()
	sup.lastStatsTime = time.Now().Add(-2 * time.Second)
	sup.statsMu.Unlock()

	res3, err := sup.QueryStats(addr)
	if err != nil {
		t.Fatalf("QueryStats fresh failed: %v", err)
	}
	if res3["u1"].Up != 99999 {
		t.Fatalf("expected fresh userUp 99999, got %d", res3["u1"].Up)
	}
	if mock.queries.Load() != 2 {
		t.Fatalf("expected mock queries to be 2, got %d", mock.queries.Load())
	}
}

func TestSupervisorPingAPIGRPC(t *testing.T) {
	mock := &mockStatsServer{}
	addr, stop := startMockGRPCServer(t, mock)

	sup := &Supervisor{}
	defer sup.closeGRPC()

	if err := sup.PingAPI(addr); err != nil {
		t.Fatalf("PingAPI failed: %v", err)
	}

	// Stop server and verify PingAPI fails
	stop()

	// Clear cached client or dial closed port
	sup.closeGRPC()
	if err := sup.PingAPI(addr); err == nil {
		t.Fatalf("expected PingAPI to fail on closed server, got nil")
	}
}

func TestSupervisorServingCheck(t *testing.T) {
	sup := &Supervisor{}
	sup.mu.Lock()
	sup.closed = true
	sup.mu.Unlock()

	_, err := sup.QueryStats("127.0.0.1:10085")
	if err == nil {
		t.Fatal("expected QueryStats to fail when closed")
	}

	sup.mu.Lock()
	sup.closed = false
	sup.suspended = true
	sup.mu.Unlock()

	_, err = sup.QueryStats("127.0.0.1:10085")
	if err == nil {
		t.Fatal("expected QueryStats to fail when suspended")
	}
}

func TestSupervisorGRPCAutoReconnect(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := lis.Addr().String()

	mock1 := &mockStatsServer{userUp: 111, userDown: 222}
	s1 := grpc.NewServer()
	statsrpc.RegisterStatsServiceServer(s1, mock1)
	go func() { _ = s1.Serve(lis) }()

	sup := &Supervisor{}
	defer sup.closeGRPC()

	res, err := sup.QueryStats(addr)
	if err != nil {
		t.Fatalf("first query failed: %v", err)
	}
	if res["u1"].Up != 111 {
		t.Fatalf("expected 111, got %d", res["u1"].Up)
	}

	// Stop s1
	s1.Stop()
	_ = lis.Close()

	// Clear cache so next query attempts RPC
	sup.statsMu.Lock()
	sup.lastStatsTime = time.Time{}
	sup.statsMu.Unlock()

	// While server is down, query should fail
	_, err = sup.QueryStats(addr)
	if err == nil {
		t.Fatal("expected query to fail while server is down")
	}

	// Start s2 on the exact same port
	lis2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("failed to re-listen on %s: %v", addr, err)
	}
	defer lis2.Close()

	mock2 := &mockStatsServer{userUp: 777, userDown: 888}
	s2 := grpc.NewServer()
	statsrpc.RegisterStatsServiceServer(s2, mock2)
	defer s2.Stop()
	go func() { _ = s2.Serve(lis2) }()

	// Clear cache again
	sup.statsMu.Lock()
	sup.lastStatsTime = time.Time{}
	sup.statsMu.Unlock()

	// Query again: gRPC client should automatically reconnect!
	res2, err := sup.QueryStats(addr)
	if err != nil {
		t.Fatalf("reconnected query failed: %v", err)
	}
	if res2["u1"].Up != 777 {
		t.Fatalf("expected 777 after reconnect, got %d", res2["u1"].Up)
	}
}

func BenchmarkParseProtoStats(b *testing.B) {
	stats := []*statsrpc.Stat{
		{Name: "user>>>u1>>>traffic>>>uplink", Value: 1048576},
		{Name: "user>>>u1>>>traffic>>>downlink", Value: 2097152},
		{Name: "user>>>u2>>>traffic>>>uplink", Value: 512},
		{Name: "user>>>u2>>>traffic>>>downlink", Value: 1024},
		{Name: "inbound>>>api>>>traffic>>>downlink", Value: 100},
		{Name: "user>>>invalid", Value: 0},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = parseProtoStats(stats)
	}
}
