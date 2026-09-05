package store

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestLegacyForkMigrationsUpgrade simulates an existing database that was created
// with earlier fork versions where node_rentals, rent_master_host, reality_default_off,
// and happ_subscriptions were applied as 0062..0065.
// It verifies that Open() safely migrates schema_migrations records, applies upstream
// 0062..0065, and skips 0066..0069 without failing on duplicate columns or tables.
func TestLegacyForkMigrationsUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy_upgrade.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}

	// 1. Apply all migrations up to 0061
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && e.Name() < "0062" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}

	// 2. Apply fork migrations 0066..0069 under their OLD names (0062..0065)
	legacyMap := map[string]string{
		"0066_node_rentals.sql":        "0062_node_rentals.sql",
		"0067_rent_master_host.sql":    "0063_rent_master_host.sql",
		"0068_reality_default_off.sql": "0064_reality_default_off.sql",
		"0069_happ_subscriptions.sql":  "0065_happ_subscriptions.sql",
	}
	for newName, oldName := range legacyMap {
		body, err := migrationsFS.ReadFile("migrations/" + newName)
		if err != nil {
			t.Fatalf("read %s: %v", newName, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply legacy %s: %v", oldName, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, oldName); err != nil {
			t.Fatalf("record legacy %s: %v", oldName, err)
		}
	}
	db.Close()

	// 3. Open via Store.Open() — this must run s.migrate() without error
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open legacy database failed: %v", err)
	}
	defer st.Close()

	// 4. Verify columns from upstream 0062..0065 are present
	var colCount int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('settings') WHERE name = 'sub_hide_offline'`).Scan(&colCount); err != nil || colCount != 1 {
		t.Errorf("upstream 0062_sub_hide_offline was not applied: colCount=%d, err=%v", colCount, err)
	}

	if err := st.db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('settings') WHERE name = 'conn_policy'`).Scan(&colCount); err != nil || colCount != 1 {
		t.Errorf("upstream 0063_conn_policy was not applied: colCount=%d, err=%v", colCount, err)
	}

	if err := st.db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('settings') WHERE name = 'abuse_warn_min'`).Scan(&colCount); err != nil || colCount != 1 {
		t.Errorf("upstream 0064_abuse_measures was not applied: colCount=%d, err=%v", colCount, err)
	}

	// 5. Verify fork migration 0072 safely dropped share_enabled and happ_subscriptions table is present
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('nodes') WHERE name = 'share_enabled'`).Scan(&colCount); err != nil || colCount != 0 {
		t.Errorf("node_rentals share_enabled was not dropped by 0072: colCount=%d, err=%v", colCount, err)
	}

	var tableCount int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = 'happ_subscriptions'`).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Errorf("happ_subscriptions table missing: tableCount=%d, err=%v", tableCount, err)
	}

	// 6. Verify schema_migrations has new versions and not old versions
	for _, oldName := range legacyMap {
		var cnt int
		_ = st.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, oldName).Scan(&cnt)
		if cnt != 0 {
			t.Errorf("old version %s still present in schema_migrations", oldName)
		}
	}
	for newName := range legacyMap {
		var cnt int
		_ = st.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, newName).Scan(&cnt)
		if cnt != 1 {
			t.Errorf("new version %s missing from schema_migrations", newName)
		}
	}
}

// TestLegacyForkMigrationsResumeAfterFailed0066 reproduces the exact failure mode observed
// on user installations where upstream 0062..0065 were applied, but 0066_node_rentals.sql
// crashed at boot due to duplicate column share_enabled. It asserts Open() recovers gracefully.
func TestLegacyForkMigrationsResumeAfterFailed0066(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume_crashed.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}

	// 1. Apply all migrations up to 0065 (including upstream 0062..0065)
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && e.Name() <= "0065_login_alert_on.sql" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}

	// 2. Manually add column share_enabled (simulating prior fork run without 0066 record in schema_migrations)
	if _, err := db.Exec(`ALTER TABLE nodes ADD COLUMN share_enabled INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("add share_enabled: %v", err)
	}
	db.Close()

	// 3. Open via Store.Open() — must NOT crash with duplicate column name
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open crashed database failed: %v", err)
	}
	defer st.Close()

	// 4. Verify 0066_node_rentals.sql is now recorded in schema_migrations
	var count66 int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = '0066_node_rentals.sql'`).Scan(&count66); err != nil || count66 != 1 {
		t.Fatalf("0066_node_rentals.sql was not recorded: count=%d, err=%v", count66, err)
	}
}
