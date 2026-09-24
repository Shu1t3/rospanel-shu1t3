package store

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentFirstUseKeepsOneSessionPepper(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "rospanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.db.Exec(`UPDATE settings SET session_pepper = '' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	const callers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan string, callers)
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			pepper, err := st.sessionPepper()
			results <- pepper
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var stored string
	if err := st.db.QueryRow(`SELECT session_pepper FROM settings WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" {
		t.Fatal("session pepper was not persisted")
	}
	for got := range results {
		if got != stored {
			t.Fatalf("caller hashed with uncommitted pepper %q, stored %q", got, stored)
		}
	}
}
