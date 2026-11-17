package store

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestReproduceUserMtprotoDuplicateColumnBug reproduces the exact user issue:
// A database that already has mtproto_enabled (or partial mtproto columns)
// but does NOT have 0126_mtproto.sql recorded in schema_migrations.
func TestReproduceUserMtprotoDuplicateColumnBug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatal(err)
	}

	// Apply migrations up to 0125
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && e.Name() < "0126" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}

	// Simulate pre-existing mtproto_enabled column (e.g. from previous run, restored db, or candidate snapshot)
	if _, err := db.Exec(`ALTER TABLE settings ADD COLUMN mtproto_enabled INTEGER NOT NULL DEFAULT 1`); err != nil {
		t.Fatalf("pre-add mtproto_enabled: %v", err)
	}
	db.Close()

	// Now try to open the store — this should succeed without "duplicate column name" error!
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed with error: %v", err)
	}
	defer st.Close()
}

// TestDuplicateColumnFallbackAnyMigration verifies that if any migration hits a duplicate column error,
// it gracefully skips duplicate columns and applies all other statements in that migration.
func TestDuplicateColumnFallbackAnyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel_anytls.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatal(err)
	}

	// Apply migrations up to 0122
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && e.Name() < "0123" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}

	// Pre-add anytls_enabled to settings without 0123 recorded
	if _, err := db.Exec(`ALTER TABLE settings ADD COLUMN anytls_enabled INTEGER NOT NULL DEFAULT 1`); err != nil {
		t.Fatalf("pre-add anytls_enabled: %v", err)
	}
	db.Close()

	// Open store via store.Open — migration 0123 must not fail on duplicate column name
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed on 0123 fallback: %v", err)
	}
	defer st.Close()

	// Verify anytls_name (second statement in 0123) and nodes columns were created
	var count int
	if err := st.db.QueryRow(`SELECT count(1) FROM pragma_table_info('settings') WHERE name = 'anytls_name'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("anytls_name missing: count=%d, err=%v", count, err)
	}
	if err := st.db.QueryRow(`SELECT count(1) FROM pragma_table_info('nodes') WHERE name = 'serves_anytls'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("serves_anytls missing: count=%d, err=%v", count, err)
	}
}

// TestTableAlreadyExistsFallbackAnyMigration verifies that if a table in a migration
// was already created (e.g. from an earlier snapshot or partial execution), s.migrate()
// skips the already existing table and applies the rest of the statements.
func TestTableAlreadyExistsFallbackAnyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rospanel_table_exists.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatal(err)
	}

	// Apply migrations up to 0033
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && e.Name() < "0034" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}

	// Pre-create 'inbounds' table from 0034 without 0034 recorded in schema_migrations
	if _, err := db.Exec(`CREATE TABLE inbounds (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id  INTEGER NOT NULL DEFAULT 0,
		enabled    INTEGER NOT NULL DEFAULT 1,
		sort       INTEGER NOT NULL DEFAULT 0,
		name       TEXT    NOT NULL,
		protocol   TEXT    NOT NULL,
		port       INTEGER NOT NULL,
		opts       TEXT    NOT NULL DEFAULT '{}',
		created_at INTEGER NOT NULL DEFAULT (unixepoch())
	)`); err != nil {
		t.Fatalf("pre-create inbounds table: %v", err)
	}
	db.Close()

	// Open store via store.Open — migration 0034 contains CREATE TABLE inbounds (bare)
	// along with groups, group_members, group_grants. It must not fail on "table inbounds already exists".
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed on table already exists fallback: %v", err)
	}
	defer st.Close()

	// Verify groups table (a subsequent statement in 0034) was created successfully
	var count int
	if err := st.db.QueryRow(`SELECT count(1) FROM sqlite_master WHERE type = 'table' AND name = 'groups'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("groups table missing: count=%d, err=%v", count, err)
	}
}
