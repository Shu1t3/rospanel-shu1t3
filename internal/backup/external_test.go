package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptDecryptFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "plain.txt")
	enc := filepath.Join(tmpDir, "enc.bin")
	dec := filepath.Join(tmpDir, "dec.txt")

	secretData := []byte("secret database contents and keys 12345")
	if err := os.WriteFile(src, secretData, 0o600); err != nil {
		t.Fatalf("write plain: %v", err)
	}

	passphrase := "my-strong-backup-passphrase-99"

	if err := EncryptFile(src, enc, passphrase); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	// Verify file is indeed encrypted
	encData, _ := os.ReadFile(enc)
	if string(encData) == string(secretData) {
		t.Error("encrypted file matches plaintext")
	}

	// Decrypt with correct passphrase
	if err := DecryptFile(enc, dec, passphrase); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	decData, _ := os.ReadFile(dec)
	if string(decData) != string(secretData) {
		t.Errorf("decrypted data mismatch: got %q, want %q", string(decData), string(secretData))
	}

	// Decrypt with wrong passphrase must fail
	decWrong := filepath.Join(tmpDir, "dec-wrong.txt")
	if err := DecryptFile(enc, decWrong, "wrong-passphrase"); err == nil {
		t.Error("expected error with wrong passphrase, got nil")
	}
}

func TestEstimateRPO(t *testing.T) {
	now := time.Now()
	// 45 minutes ago
	_, s1 := EstimateRPO(now.Add(-45 * time.Minute))
	if s1 == "" || s1 == "неизвестно (резервные копии отсутствуют)" {
		t.Errorf("unexpected RPO string: %s", s1)
	}

	// Zero time
	_, sZero := EstimateRPO(time.Time{})
	if sZero != "неизвестно (резервные копии отсутствуют)" {
		t.Errorf("expected missing backup notice, got %s", sZero)
	}
}
