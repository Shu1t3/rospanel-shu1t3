package model

import (
	"fmt"
	"net/url"
	"strings"
)

// DefaultFakeTLSDomain is the standard fallback fronting SNI.
const DefaultFakeTLSDomain = "cloudflare.com"

// MTProtoConfig defines the embedded MTProto-proxy configuration on a node (mixed mode)
// or master server.
type MTProtoConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Secret   string `json:"secret"`
	Domain   string `json:"domain"`
	MaxConns int    `json:"max_conns"`
}

// Normalize sets safe defaults for an MTProtoConfig.
func (c *MTProtoConfig) Normalize() {
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = 8443
	}
	if strings.TrimSpace(c.Domain) == "" {
		c.Domain = DefaultFakeTLSDomain
	}
	if c.MaxConns <= 0 {
		c.MaxConns = 512
	}
}

// MTProtoProxy represents an MTProto proxy instance shown on the "Servers" tab.
// It represents either a standalone proxy (deployed on Pterodactyl / container) or
// a node running in mixed mode.
type MTProtoProxy struct {
	ID        int64  `json:"id"`
	NodeID    *int64 `json:"node_id,omitempty"` // nil for standalone, or points to NodeID for mixed-mode node
	NodeName  string `json:"node_name,omitempty"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Secret    string `json:"secret"`
	Domain    string `json:"domain"`
	MaxConns  int    `json:"max_conns"`
	Enabled   bool   `json:"enabled"`
	Token     string `json:"token,omitempty"` // bearer token for standalone proxy sync/heartbeat
	CreatedAt int64  `json:"created_at"`

	// Dynamic runtime statistics reported by heartbeat or nodeagent
	Running      bool   `json:"running"`
	ActiveConns  int64  `json:"active_conns"`
	BytesRead    uint64 `json:"bytes_read"`
	BytesWritten uint64 `json:"bytes_written"`
	LastSeen     int64  `json:"last_seen"`
	UptimeSec    int64  `json:"uptime_sec"`
	MemAlloc     uint64 `json:"mem_alloc"`
	RSS          uint64 `json:"rss"`
	LastError    string `json:"last_error,omitempty"`
}

// Link returns the tg:// proxy link.
func (p MTProtoProxy) Link() string {
	q := url.Values{}
	q.Set("server", p.Host)
	q.Set("port", fmt.Sprintf("%d", p.Port))
	q.Set("secret", strings.TrimSpace(p.Secret))
	return "tg://proxy?" + q.Encode()
}

// HTTPSLink returns the https://t.me/proxy link.
func (p MTProtoProxy) HTTPSLink() string {
	q := url.Values{}
	q.Set("server", p.Host)
	q.Set("port", fmt.Sprintf("%d", p.Port))
	q.Set("secret", strings.TrimSpace(p.Secret))
	return "https://t.me/proxy?" + q.Encode()
}
