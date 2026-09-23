package migration

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/datasec"
	_ "modernc.org/sqlite"
)

func TestConsistentSnapshotCreationAndValidation(t *testing.T) {
	dataDir := t.TempDir()

	// 1. Setup mock database
	dbPath := filepath.Join(dataDir, "rospanel.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at INTEGER);
		INSERT INTO schema_migrations (version, applied_at) VALUES ('0050_test.sql', 1700000000);
		CREATE TABLE settings (id INTEGER PRIMARY KEY, host TEXT, panel_secret_path TEXT, sub_path TEXT);
		INSERT INTO settings (id, host, panel_secret_path, sub_path) VALUES (1, 'vpn.test.local', 'secret-abc', 'mysub');
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob');
		CREATE TABLE admins (id INTEGER PRIMARY KEY, username TEXT);
		INSERT INTO admins (id, username) VALUES (1, 'admin');
		CREATE TABLE nodes (id INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO nodes (id, name) VALUES (1, 'node-us');
	`)
	if err != nil {
		t.Fatalf("setup tables: %v", err)
	}
	_ = db.Close()

	// 2. Setup secrets.key
	if err := datasec.Init(dataDir); err != nil {
		t.Fatalf("datasec.Init: %v", err)
	}

	// 3. Create snapshot
	archivePath := filepath.Join(dataDir, "snapshot.tar.gz")
	manifest, err := CreateConsistentSnapshot(dataDir, archivePath)
	if err != nil {
		t.Fatalf("CreateConsistentSnapshot: %v", err)
	}

	if manifest.UsersCount != 2 || manifest.AdminsCount != 1 || manifest.NodesCount != 1 {
		t.Errorf("manifest counts mismatch: %+v", manifest)
	}
	if manifest.PublicDomain != "vpn.test.local" || manifest.SecretPath != "secret-abc" {
		t.Errorf("manifest settings mismatch: %+v", manifest)
	}
	if manifest.DatabaseSHA256 == "" {
		t.Error("expected non-empty DatabaseSHA256")
	}

	// 4. Validate & Extract snapshot into a clean targetDir
	targetDir := t.TempDir()
	extractedManifest, err := ValidateAndExtractSnapshot(archivePath, targetDir, "0050_test.sql")
	if err != nil {
		t.Fatalf("ValidateAndExtractSnapshot: %v", err)
	}
	if extractedManifest.UsersCount != manifest.UsersCount {
		t.Errorf("extracted users mismatch: got %d, want %d", extractedManifest.UsersCount, manifest.UsersCount)
	}

	// Verify extracted files exist
	if _, err := os.Stat(filepath.Join(targetDir, "rospanel.db")); err != nil {
		t.Errorf("extracted rospanel.db missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "secrets.key")); err != nil {
		t.Errorf("extracted secrets.key missing: %v", err)
	}

	// Verify schema compatibility check
	targetDirFail := t.TempDir()
	if _, err := ValidateAndExtractSnapshot(archivePath, targetDirFail, "0040_older.sql"); err == nil {
		t.Error("expected error due to incompatible newer schema in snapshot, got nil")
	}
}
