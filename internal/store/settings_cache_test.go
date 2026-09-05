package store_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
)

func TestSettingsCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "settings_cache.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	// 1. Initial read (fills cache)
	s1, err := st.GetSettings()
	if err != nil {
		t.Fatalf("first GetSettings: %v", err)
	}

	// 2. Second read (hits cache)
	s2, err := st.GetSettings()
	if err != nil {
		t.Fatalf("second GetSettings: %v", err)
	}

	// Must be different pointer instances (deep copy protection)
	if s1 == s2 {
		t.Fatal("GetSettings must return a distinct pointer on each call to prevent races")
	}

	// Mutating s1 must not affect s2 or subsequent reads
	s1.MasterLabel = "MUTATED_TEST"
	s3, err := st.GetSettings()
	if err != nil {
		t.Fatalf("third GetSettings: %v", err)
	}
	if s3.MasterLabel == "MUTATED_TEST" {
		t.Fatal("mutation of previously returned Settings corrupted cached settings")
	}

	// 3. Setter invalidation: SetMasterLabel
	if err := st.SetMasterLabel("NewBrandName"); err != nil {
		t.Fatalf("SetMasterLabel: %v", err)
	}
	s4, err := st.GetSettings()
	if err != nil {
		t.Fatalf("fourth GetSettings: %v", err)
	}
	if s4.MasterLabel != "NewBrandName" {
		t.Fatalf("expected MasterLabel 'NewBrandName' after invalidation, got %q", s4.MasterLabel)
	}

	// 4. Invalidation on SetConnPolicy
	p := model.ConnPolicy{Mode: model.ConnPolicyAllow, Countries: []string{"US", "DE"}}
	if err := st.SetConnPolicy(p); err != nil {
		t.Fatalf("SetConnPolicy: %v", err)
	}
	s5, err := st.GetSettings()
	if err != nil {
		t.Fatalf("fifth GetSettings: %v", err)
	}
	if s5.ConnPolicy.Mode != model.ConnPolicyAllow || len(s5.ConnPolicy.Countries) != 2 {
		t.Fatalf("expected ConnPolicy to be updated after SetConnPolicy, got %+v", s5.ConnPolicy)
	}

	// 5. Concurrency test: ensure no data races between concurrent GetSettings and SetMasterLabel
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = st.GetSettings()
				if j%10 == 0 {
					_ = st.SetMasterLabel("BrandConcurrent")
				}
			}
		}(i)
	}
	wg.Wait()
}
