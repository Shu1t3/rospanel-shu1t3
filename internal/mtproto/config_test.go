package mtproto

import (
	"os"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error validating config with empty secret")
	}

	sec, err := GenerateSecret("google.com")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	cfg.Secret = sec
	cfg.Port = 8443

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	// Test invalid port
	cfg.Port = 99999
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error with out-of-range port")
	}
}

func TestConfigHashDeterministic(t *testing.T) {
	cfg1 := DefaultConfig()
	cfg1.Secret = "ee0102030405060708090a0b0c0d0e0f676f6f676c652e636f6d"
	cfg1.Port = 8443
	_ = cfg1.Validate()

	cfg2 := DefaultConfig()
	cfg2.Secret = cfg1.Secret
	cfg2.Port = cfg1.Port
	_ = cfg2.Validate()

	if cfg1.ConfigHash() != cfg2.ConfigHash() {
		t.Fatalf("expected deterministic hash: %s != %s", cfg1.ConfigHash(), cfg2.ConfigHash())
	}

	cfg2.MaxConns = 1024
	if cfg1.ConfigHash() == cfg2.ConfigHash() {
		t.Fatal("expected different hash when MaxConns changed")
	}
}

func TestConfigFromEnv(t *testing.T) {
	os.Setenv("MTPROTO_PORT", "9443")
	os.Setenv("MTPROTO_SECRET", "ee112233445566778899aabbccddeeff676f6f676c652e636f6d")
	os.Setenv("MTPROTO_MAX_CONNS", "256")
	os.Setenv("MTPROTO_ANTI_REPLAY", "1")
	defer func() {
		os.Unsetenv("MTPROTO_PORT")
		os.Unsetenv("MTPROTO_SECRET")
		os.Unsetenv("MTPROTO_MAX_CONNS")
		os.Unsetenv("MTPROTO_ANTI_REPLAY")
	}()

	cfg := DefaultConfig()
	cfg.FromEnv()

	if cfg.Port != 9443 {
		t.Fatalf("expected port 9443, got %d", cfg.Port)
	}
	if cfg.MaxConns != 256 {
		t.Fatalf("expected MaxConns 256, got %d", cfg.MaxConns)
	}
	if !cfg.AntiReplay {
		t.Fatal("expected AntiReplay true")
	}
}

func TestParseJSON(t *testing.T) {
	raw := []byte(`{
		"bind_addr": "127.0.0.1:8443",
		"port": 8443,
		"secret": "ee00112233445566778899aabbccddeeff636c6f7564666c6172652e636f6d",
		"max_conns": 300,
		"idle_timeout": 60000000000
	}`)

	cfg, err := ParseJSON(raw)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if cfg.Port != 8443 || cfg.MaxConns != 300 || cfg.IdleTimeout != time.Minute {
		t.Fatalf("parsed values mismatch: %+v", cfg)
	}
}
