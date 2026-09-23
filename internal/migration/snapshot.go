package migration

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/datasec"
	"github.com/Shu1t3/rospanel-shu1t3/internal/version"
	_ "modernc.org/sqlite"
)

// SnapshotManifest describes metadata and validation metrics of a migration snapshot.
type SnapshotManifest struct {
	Version        string `json:"version"`
	PublicDomain   string `json:"public_domain"`
	SecretPath     string `json:"secret_path"`
	SubPath        string `json:"sub_path"`
	CreatedAt      string `json:"created_at"`
	UsersCount     int    `json:"users_count"`
	AdminsCount    int    `json:"admins_count"`
	NodesCount     int    `json:"nodes_count"`
	SchemaVersion  string `json:"schema_version"`
	DatabaseSHA256 string `json:"database_sha256"`
}

// CreateConsistentSnapshot flushes WAL to disk, verifies SQLite integrity,
// exports a clean consistent database snapshot via VACUUM INTO, and bundles
// DB, secrets.key, and certificates into a verified tar.gz.
func CreateConsistentSnapshot(dataDir string, outTarGz string) (*SnapshotManifest, error) {
	dbPath := filepath.Join(dataDir, "rospanel.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found: %w", err)
	}

	// 1. Open writer connection to force WAL checkpoint and integrity check
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)", dbPath))
	if err != nil {
		return nil, fmt.Errorf("open db for snapshot: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return nil, fmt.Errorf("checkpoint WAL: %w", err)
	}

	var quickResult string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&quickResult); err != nil || !strings.EqualFold(quickResult, "ok") {
		return nil, fmt.Errorf("pre-snapshot integrity check failed: %s (err: %v)", quickResult, err)
	}

	// 2. Export clean database using VACUUM INTO to a temp file
	tmpDB, err := os.CreateTemp(dataDir, "snapshot-db-*.db")
	if err != nil {
		return nil, err
	}
	tmpDBPath := tmpDB.Name()
	_ = tmpDB.Close()
	_ = os.Remove(tmpDBPath) // VACUUM INTO expects target to not exist
	defer os.Remove(tmpDBPath)

	vacuumQuery := fmt.Sprintf("VACUUM INTO '%s'", tmpDBPath)
	if _, err := db.Exec(vacuumQuery); err != nil {
		// Fallback to direct locked copy if VACUUM INTO fails
		if err := copyFile(dbPath, tmpDBPath); err != nil {
			return nil, fmt.Errorf("export database: %w", err)
		}
	}

	// 3. Inspect exported DB
	expDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", tmpDBPath))
	if err != nil {
		return nil, fmt.Errorf("open exported db: %w", err)
	}
	defer expDB.Close()

	manifest := &SnapshotManifest{
		Version:   version.Version,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	_ = expDB.QueryRow(`SELECT count(*) FROM users`).Scan(&manifest.UsersCount)
	_ = expDB.QueryRow(`SELECT count(*) FROM admins`).Scan(&manifest.AdminsCount)
	_ = expDB.QueryRow(`SELECT count(*) FROM nodes`).Scan(&manifest.NodesCount)
	_ = expDB.QueryRow(`SELECT host, panel_secret_path, sub_path FROM settings WHERE id = 1`).Scan(
		&manifest.PublicDomain, &manifest.SecretPath, &manifest.SubPath)
	_ = expDB.QueryRow(`SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&manifest.SchemaVersion)

	// Calculate SHA256 of the exported database
	dbFile, err := os.Open(tmpDBPath)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, dbFile); err != nil {
		_ = dbFile.Close()
		return nil, err
	}
	_ = dbFile.Close()
	manifest.DatabaseSHA256 = hex.EncodeToString(h.Sum(nil))

	// 4. Create output archive
	tmpArchive, err := os.CreateTemp(filepath.Dir(outTarGz), "snapshot-*.tar.gz.tmp")
	if err != nil {
		return nil, err
	}
	tmpArchivePath := tmpArchive.Name()
	defer os.Remove(tmpArchivePath)

	gw := gzip.NewWriter(tmpArchive)
	tw := tar.NewWriter(gw)

	// Write manifest.json
	mData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = tw.Close()
		_ = gw.Close()
		_ = tmpArchive.Close()
		return nil, err
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:    "manifest.json",
		Mode:    0o600,
		Size:    int64(len(mData)),
		ModTime: time.Now(),
	}); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		_ = tmpArchive.Close()
		return nil, err
	}
	if _, err := tw.Write(mData); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		_ = tmpArchive.Close()
		return nil, err
	}

	// Write rospanel.db
	if err := addFileToTar(tw, tmpDBPath, "rospanel.db"); err != nil {
		_ = tw.Close()
		_ = gw.Close()
		_ = tmpArchive.Close()
		return nil, err
	}

	// Write secrets.key if present
	secretKeyPath := filepath.Join(dataDir, "secrets.key")
	if _, err := os.Stat(secretKeyPath); err == nil {
		if err := addFileToTar(tw, secretKeyPath, "secrets.key"); err != nil {
			_ = tw.Close()
			_ = gw.Close()
			_ = tmpArchive.Close()
			return nil, err
		}
	}

	// Write certs/ directory if present
	certsDir := filepath.Join(dataDir, "certs")
	if fi, err := os.Stat(certsDir); err == nil && fi.IsDir() {
		_ = filepath.Walk(certsDir, func(path string, info os.FileInfo, werr error) error {
			if werr != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dataDir, path)
			return addFileToTar(tw, path, rel)
		})
	}

	// Write acme/ directory if present
	acmeDir := filepath.Join(dataDir, "acme")
	if fi, err := os.Stat(acmeDir); err == nil && fi.IsDir() {
		_ = filepath.Walk(acmeDir, func(path string, info os.FileInfo, werr error) error {
			if werr != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dataDir, path)
			return addFileToTar(tw, path, rel)
		})
	}

	if err := tw.Close(); err != nil {
		_ = gw.Close()
		_ = tmpArchive.Close()
		return nil, err
	}
	if err := gw.Close(); err != nil {
		_ = tmpArchive.Close()
		return nil, err
	}
	if err := tmpArchive.Close(); err != nil {
		return nil, err
	}

	if err := os.Rename(tmpArchivePath, outTarGz); err != nil {
		return nil, err
	}
	return manifest, nil
}

// ValidateAndExtractSnapshot verifies snapshot integrity, schema compatibility,
// and secret decryption before unpacking into targetDir.
func ValidateAndExtractSnapshot(archivePath, targetDir string, maxSupportedSchema string) (*SnapshotManifest, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("invalid gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	var manifest SnapshotManifest
	manifestFound := false
	stagingDir := filepath.Join(targetDir, ".snapshot_staging")
	_ = os.RemoveAll(stagingDir)
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return nil, err
	}
	defer os.RemoveAll(stagingDir)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return nil, fmt.Errorf("unsafe archive path: %s", hdr.Name)
		}

		if cleanName == "manifest.json" {
			if err := json.NewDecoder(tr).Decode(&manifest); err != nil {
				return nil, fmt.Errorf("invalid manifest.json: %w", err)
			}
			manifestFound = true
			continue
		}

		dstPath := filepath.Join(stagingDir, cleanName)
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(dstPath, 0o700); err != nil {
				return nil, err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
			return nil, err
		}
		out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return nil, err
		}
		_ = out.Close()
	}

	if !manifestFound {
		return nil, errors.New("manifest.json missing from snapshot")
	}

	// 1. Verify DB SHA256
	stagedDB := filepath.Join(stagingDir, "rospanel.db")
	dbFile, err := os.Open(stagedDB)
	if err != nil {
		return nil, fmt.Errorf("rospanel.db missing from snapshot: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, dbFile); err != nil {
		_ = dbFile.Close()
		return nil, err
	}
	_ = dbFile.Close()
	actualHash := hex.EncodeToString(h.Sum(nil))
	if manifest.DatabaseSHA256 != "" && actualHash != manifest.DatabaseSHA256 {
		return nil, fmt.Errorf("database checksum mismatch: expected %s, got %s", manifest.DatabaseSHA256, actualHash)
	}

	// 2. Check DB quick_check
	testDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", stagedDB))
	if err != nil {
		return nil, fmt.Errorf("open staged db: %w", err)
	}
	var res string
	checkErr := testDB.QueryRow(`PRAGMA quick_check(1)`).Scan(&res)
	_ = testDB.Close()
	if checkErr != nil || !strings.EqualFold(res, "ok") {
		return nil, fmt.Errorf("staged db integrity check failed: %s (%v)", res, checkErr)
	}

	// 3. Check schema version
	if maxSupportedSchema != "" && manifest.SchemaVersion > maxSupportedSchema {
		return nil, fmt.Errorf("схема снимка (%s) новее поддерживаемой этим сервером (%s)", manifest.SchemaVersion, maxSupportedSchema)
	}

	// 4. Check secrets decryption if secrets.key is present
	stagedKey := filepath.Join(stagingDir, "secrets.key")
	if _, err := os.Stat(stagedKey); err == nil {
		if err := datasec.Init(stagingDir); err != nil {
			return nil, fmt.Errorf("failed to initialize and test secrets.key: %w", err)
		}
	}

	// 5. Apply staging to targetDir
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		src := filepath.Join(stagingDir, e.Name())
		dst := filepath.Join(targetDir, e.Name())
		_ = os.RemoveAll(dst)
		if err := os.Rename(src, dst); err != nil {
			return nil, fmt.Errorf("apply snapshot file %s: %w", e.Name(), err)
		}
	}

	return &manifest, nil
}

func addFileToTar(tw *tar.Writer, filePath, tarName string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(tarName)
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
