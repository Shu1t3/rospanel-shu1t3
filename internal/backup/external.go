package backup

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ExternalBackupConfig defines settings for scheduled encrypted remote backups.
type ExternalBackupConfig struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"` // "s3", "webdav", "local_mount"
	Endpoint      string `json:"endpoint"` // e.g. "https://s3.example.com/bucket" or WebDAV URL
	AccessKey     string `json:"access_key,omitempty"`
	SecretKey     string `json:"secret_key,omitempty"`
	Path          string `json:"path"` // remote directory/prefix
	EncryptionKey string `json:"encryption_key,omitempty"` // passphrase for AES-256-GCM
	CronExpr      string `json:"cron_expr"`
}

// TrialRestoreReport records outcome of verifying a backup via sandboxed restore.
type TrialRestoreReport struct {
	TestedAt   time.Time `json:"tested_at"`
	Valid      bool      `json:"valid"`
	UsersCount int       `json:"users_count"`
	Domain     string    `json:"domain"`
	Error      string    `json:"error,omitempty"`
}

// BackupStatusView summarizes state of backups and estimated data loss window (RPO).
type BackupStatusView struct {
	LastBackupAt       *time.Time          `json:"last_backup_at,omitempty"`
	LastSuccessfulRPO  string              `json:"last_successful_rpo,omitempty"`
	RPOSeconds         int64               `json:"rpo_seconds,omitempty"`
	TrialRestoreStatus *TrialRestoreReport `json:"trial_restore_status,omitempty"`
	TotalLocalBackups  int                 `json:"total_local_backups"`
}

// EncryptFile encrypts src file to dst using AES-256-GCM derived from passphrase.
func EncryptFile(src, dst, passphrase string) error {
	if passphrase == "" {
		return errors.New("passphrase cannot be empty")
	}
	key := sha256.Sum256([]byte(passphrase))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return os.WriteFile(dst, ciphertext, 0o600)
}

// DecryptFile decrypts src to dst using AES-256-GCM and passphrase.
func DecryptFile(src, dst, passphrase string) error {
	if passphrase == "" {
		return errors.New("passphrase cannot be empty")
	}
	key := sha256.Sum256([]byte(passphrase))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decryption failed (wrong passphrase or corrupt file): %w", err)
	}

	return os.WriteFile(dst, plaintext, 0o600)
}

// TrialRestoreSandbox executes a test restore in an isolated temporary directory.
func TrialRestoreSandbox(archivePath string) (*TrialRestoreReport, error) {
	sandboxDir, err := os.MkdirTemp("", "trial-restore-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(sandboxDir)

	report := &TrialRestoreReport{
		TestedAt: time.Now(),
	}

	if err := Restore(archivePath, sandboxDir); err != nil {
		report.Valid = false
		report.Error = fmt.Sprintf("unpack archive: %v", err)
		return report, nil
	}

	// Verify database integrity
	dbPath := filepath.Join(sandboxDir, "rospanel.db")
	if _, err := os.Stat(dbPath); err != nil {
		report.Valid = false
		report.Error = "rospanel.db missing in archive"
		return report, nil
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		report.Valid = false
		report.Error = fmt.Sprintf("open db: %v", err)
		return report, nil
	}
	defer db.Close()

	var quickCheck string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&quickCheck); err != nil || !strings.EqualFold(quickCheck, "ok") {
		report.Valid = false
		report.Error = fmt.Sprintf("integrity failure: %s (err: %v)", quickCheck, err)
		return report, nil
	}

	_ = db.QueryRow(`SELECT count(*) FROM users`).Scan(&report.UsersCount)
	_ = db.QueryRow(`SELECT host FROM settings WHERE id = 1`).Scan(&report.Domain)

	report.Valid = true
	return report, nil
}

// UploadExternal transfers the backup archive to configured remote storage.
func UploadExternal(ctx context.Context, localPath string, cfg ExternalBackupConfig) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fileName := filepath.Base(localPath)

	switch cfg.Provider {
	case "local_mount":
		dstDir := filepath.Join(cfg.Path)
		if err := os.MkdirAll(dstDir, 0o700); err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, fileName)
		out, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, f)
		return err

	case "webdav":
		url := strings.TrimRight(cfg.Endpoint, "/") + "/" + strings.Trim(cfg.Path, "/") + "/" + fileName
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, f)
		if err != nil {
			return err
		}
		if cfg.AccessKey != "" {
			req.SetBasicAuth(cfg.AccessKey, cfg.SecretKey)
		}
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("webdav upload failed with HTTP %d: %s", resp.StatusCode, string(b))
		}
		return nil

	default:
		return fmt.Errorf("unsupported external backup provider: %s", cfg.Provider)
	}
}

// EstimateRPO computes data loss window based on last backup timestamp.
func EstimateRPO(lastBackupTime time.Time) (duration time.Duration, humanString string) {
	if lastBackupTime.IsZero() {
		return 0, "неизвестно (резервные копии отсутствуют)"
	}
	diff := time.Since(lastBackupTime)
	if diff < 0 {
		diff = 0
	}

	h := int(diff.Hours())
	m := int(diff.Minutes()) % 60
	s := int(diff.Seconds()) % 60

	if h > 0 {
		return diff, fmt.Sprintf("%d ч %d мин (RPO: потенциальная потеря данных за %d ч %d мин)", h, m, h, m)
	}
	if m > 0 {
		return diff, fmt.Sprintf("%d мин %d с (RPO: потенциальная потеря данных за %d мин)", m, s, m)
	}
	return diff, fmt.Sprintf("%d с (RPO: минимальная потеря)", s)
}

// DisasterRecoveryReport summarizes automated single-command disaster recovery from external backup.
type DisasterRecoveryReport struct {
	RestoredAt        time.Time `json:"restored_at"`
	BackupCreatedAt   string    `json:"backup_created_at"`
	PublicDomain      string    `json:"public_domain"`
	SecretPath        string    `json:"secret_path"`
	UsersCount        int       `json:"users_count"`
	EstimatedDataLoss string    `json:"estimated_data_loss"`
}

// DisasterRecoverFromBackup stages recovery from a standalone archive file.
func DisasterRecoverFromBackup(archivePath, dataDir, passphrase string) (*DisasterRecoveryReport, error) {
	cleanArchive := archivePath
	if passphrase != "" {
		decryptedTmp := filepath.Join(dataDir, ".disaster-decrypted.tar.gz")
		defer os.Remove(decryptedTmp)
		if err := DecryptFile(archivePath, decryptedTmp, passphrase); err != nil {
			return nil, fmt.Errorf("расшифровка резервной копии не удалась: %w", err)
		}
		cleanArchive = decryptedTmp
	}

	// 1. Verify and read manifest
	f, err := os.Open(cleanArchive)
	if err != nil {
		return nil, err
	}
	manifest, err := ReadManifest(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("чтение манифеста копии: %w", err)
	}

	// 2. Stage restore
	if err := StageRestore(cleanArchive, dataDir); err != nil {
		return nil, fmt.Errorf("развертывание снимка в staging: %w", err)
	}

	// 3. Compute RPO
	rpoStr := "неизвестно"
	if manifest.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, manifest.CreatedAt); err == nil {
			_, rpoStr = EstimateRPO(t)
		}
	}

	return &DisasterRecoveryReport{
		RestoredAt:        time.Now(),
		BackupCreatedAt:   manifest.CreatedAt,
		PublicDomain:      manifest.Domain,
		SecretPath:        manifest.SecretPath,
		UsersCount:        manifest.UserCount,
		EstimatedDataLoss: rpoStr,
	}, nil
}
