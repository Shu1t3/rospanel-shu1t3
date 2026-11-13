package tlsmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/tlsutil"
)

func TestEnsureReplacesFallbackAfterHostChange(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	// A file where the ACME account directory should be forces a local failure.
	acmeDir := filepath.Join(dir, "blocked")
	if err := os.WriteFile(acmeDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	set := &model.Settings{Host: "2001:db8::7"}
	ensure := func() string {
		t.Helper()
		if err := Ensure(set, certPath, keyPath, acmeDir, false); err == nil {
			t.Fatal("expected ACME to be unavailable")
		}
		if !tlsutil.CertCovers(certPath, set.Host) {
			t.Fatalf("fallback certificate does not cover %s", set.Host)
		}
		pin, err := tlsutil.CertPinSHA256(certPath)
		if err != nil {
			t.Fatal(err)
		}
		return pin
	}

	oldPin := ensure()
	set.Host = "node.example.test"
	newPin := ensure()
	if newPin == oldPin || ensure() != newPin {
		t.Fatal("fallback certificate should rotate once for the new host")
	}
}
