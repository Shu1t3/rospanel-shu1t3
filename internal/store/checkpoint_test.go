package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointRejectsBusyWAL(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "rospanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`PRAGMA busy_timeout(50)`); err != nil {
		t.Fatal(err)
	}
	reader, err := st.rdb.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()
	var count int
	if err := reader.QueryRow(`SELECT count(*) FROM settings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE settings SET host = 'checkpoint-busy' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := st.Checkpoint(); err == nil || !strings.Contains(err.Error(), "incomplete WAL checkpoint") {
		t.Fatalf("checkpoint with held WAL reader = %v, want incomplete checkpoint error", err)
	}
}
