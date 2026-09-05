package core

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/nodeapi"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func BenchmarkNodeDesiredState(b *testing.B) {
	st, err := store.Open(filepath.Join(b.TempDir(), "bench_desired.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()

	n, err := st.CreateNode("bench_node", "bench.example.com", "nginx")
	if err != nil {
		b.Fatal(err)
	}
	yes := true
	n.VLESSEnabled = &yes

	mgr := &Manager{
		store:            st,
		nodes:            newNodeRegistry(),
		opts:             xray.Options{PanelDest: "127.0.0.1:8080"},
		tz:               time.Local,
		applied:          map[int64]struct{}{},
		nodeGeoFiles:     map[int64][]nodeapi.GeoFile{},
		nodeHostStats:    map[int64]nodeapi.HostStats{},
		nodeSyncFails:    map[int64]int{},
		nodeAWGRunning:   map[int64]bool{},
		nodeAWGErr:       map[int64]string{},
		nodeDesiredCache: make(map[int64]cachedNodeState),
	}

	// Create 50 working users
	for i := 0; i < 50; i++ {
		_, err := st.CreateUser(fmt.Sprintf("user_%d", i), fmt.Sprintf("uuid_%d", i), "pw", fmt.Sprintf("sub_%d", i), 0, 0, 0)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.Run("CacheHit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			state, err := mgr.NodeDesiredState(n)
			if err != nil {
				b.Fatal(err)
			}
			if state.Hash == "" {
				b.Fatal("empty state hash")
			}
		}
	})

	b.Run("CacheMiss", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mgr.InvalidateNodeDesiredCache(n.ID)
			state, err := mgr.NodeDesiredState(n)
			if err != nil {
				b.Fatal(err)
			}
			if state.Hash == "" {
				b.Fatal("empty state hash")
			}
		}
	})
}
