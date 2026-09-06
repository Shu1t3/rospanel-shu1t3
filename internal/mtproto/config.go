package mtproto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultMaxConns is the default concurrency limit (worker pool size).
	// With ~16-32 KB buffer footprint per connection, 512 connections consume ~16 MB RAM,
	// keeping total RSS comfortably below the 96 MB Pterodactyl container limit.
	DefaultMaxConns = 512

	// DefaultPort is the standard port for FakeTLS MTProto proxies.
	DefaultPort = 8443

	// DefaultIdleTimeout is how long an inactive connection is held before aborting.
	DefaultIdleTimeout = 2 * time.Minute

	// DefaultHandshakeTimeout bounds the client TLS/obfuscation handshake.
	DefaultHandshakeTimeout = 10 * time.Second

	// DefaultBufferSize is the relay copy buffer size in bytes.
	DefaultBufferSize = 16 * 1024

	// DefaultHeartbeatInterval is how often the control-plane reports status to the panel.
	DefaultHeartbeatInterval = 30 * time.Second
)

// Config holds runtime parameters for the MTProto proxy and its control-plane.
type Config struct {
	BindAddr          string        `json:"bind_addr"`
	Port              int           `json:"port"`
	Secret            string        `json:"secret"`
	Domain            string        `json:"domain"`
	MaxConns          uint          `json:"max_conns"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	HandshakeTimeout  time.Duration `json:"handshake_timeout"`
	BufferSize        int           `json:"buffer_size"`
	RemoteSyncURL     string        `json:"remote_sync_url"`
	SyncToken         string        `json:"sync_token"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	PprofAddr         string        `json:"pprof_addr"`
	AntiReplay        bool          `json:"anti_replay"`
}

// DefaultConfig returns a memory-optimized default configuration.
func DefaultConfig() Config {
	return Config{
		BindAddr:          ":8443",
		Port:              DefaultPort,
		MaxConns:          DefaultMaxConns,
		IdleTimeout:       DefaultIdleTimeout,
		HandshakeTimeout:  DefaultHandshakeTimeout,
		BufferSize:        DefaultBufferSize,
		HeartbeatInterval: DefaultHeartbeatInterval,
	}
}

// Validate checks configuration sanity.
func (c *Config) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 0 and 65535", c.Port)
	}
	if strings.TrimSpace(c.Secret) == "" {
		return errors.New("mtproto secret is required")
	}
	if c.Port == 0 && c.BindAddr == "" {
		c.Port = DefaultPort
	}
	if c.MaxConns == 0 {
		c.MaxConns = DefaultMaxConns
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = DefaultIdleTimeout
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if c.BufferSize <= 0 {
		c.BufferSize = DefaultBufferSize
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if c.BindAddr == "" {
		c.BindAddr = fmt.Sprintf(":%d", c.Port)
	}
	return nil
}

// ConfigHash returns a deterministic SHA-256 fingerprint of the serving configuration.
func (c *Config) ConfigHash() string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d|%s|%s|%d|%d|%d|%d|%t",
		c.BindAddr,
		c.Port,
		c.Secret,
		c.Domain,
		c.MaxConns,
		c.IdleTimeout,
		c.HandshakeTimeout,
		c.BufferSize,
		c.AntiReplay,
	)
	return hex.EncodeToString(h.Sum(nil))
}

func getEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// ParseConfigFromEnv returns a default Config with environment overrides applied.
func ParseConfigFromEnv() Config {
	cfg := DefaultConfig()
	cfg.FromEnv()
	return cfg
}

// FromEnv populates configuration fields from environment variables.
func (c *Config) FromEnv() {
	if v := getEnv("ROSPANEL_MTPROTO_BIND", "MTPROTO_BIND"); v != "" {
		c.BindAddr = v
	}
	if v := getEnv("ROSPANEL_MTPROTO_PORT", "MTPROTO_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p <= 65535 {
			c.Port = p
		}
	}
	if v := getEnv("ROSPANEL_MTPROTO_SECRET", "MTPROTO_SECRET"); v != "" {
		c.Secret = v
	}
	if v := getEnv("ROSPANEL_MTPROTO_DOMAIN", "MTPROTO_DOMAIN"); v != "" {
		c.Domain = v
	}
	if v := getEnv("ROSPANEL_MTPROTO_MAX_CONNS", "MTPROTO_MAX_CONNS"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			c.MaxConns = uint(n)
		}
	}
	if v := getEnv("ROSPANEL_MTPROTO_SYNC_URL", "MTPROTO_SYNC_URL"); v != "" {
		c.RemoteSyncURL = v
	}
	if v := getEnv("ROSPANEL_MTPROTO_SYNC_TOKEN", "MTPROTO_SYNC_TOKEN"); v != "" {
		c.SyncToken = v
	}
	if v := getEnv("ROSPANEL_PPROF_ADDR", "MTPROTO_PPROF_ADDR"); v != "" {
		c.PprofAddr = v
	}
	if v := getEnv("ROSPANEL_MTPROTO_ANTI_REPLAY", "MTPROTO_ANTI_REPLAY"); v != "" {
		c.AntiReplay = v == "1" || strings.ToLower(v) == "true"
	}
}

// ParseJSON decodes JSON bytes into Config.
func ParseJSON(data []byte) (Config, error) {
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config json: %w", err)
	}
	return cfg, nil
}
