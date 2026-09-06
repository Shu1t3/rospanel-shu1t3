package mtproto

import (
	"strings"
	"testing"
)

func TestGenerateAndParseSecret(t *testing.T) {
	domain := "cloudflare.com"
	secretHex, err := GenerateSecret(domain)
	if err != nil {
		t.Fatalf("unexpected error generating secret: %v", err)
	}

	if !strings.HasPrefix(secretHex, "ee") {
		t.Fatalf("expected FakeTLS secret to start with 'ee', got %s", secretHex)
	}

	sec, err := ParseSecret(secretHex)
	if err != nil {
		t.Fatalf("failed to parse generated secret: %v", err)
	}

	if sec.Host != domain {
		t.Fatalf("expected secret host %q, got %q", domain, sec.Host)
	}
}

func TestParseInvalidSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{"empty", ""},
		{"too short", "ee0102"},
		{"invalid hex", "eeZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSecret(tt.secret)
			if err == nil {
				t.Fatalf("expected error parsing invalid secret %q, got nil", tt.secret)
			}
		})
	}
}

func TestTelegramLinks(t *testing.T) {
	host := "vpn.example.com"
	port := 8443
	secret := "ee0102030405060708090a0b0c0d0e0f636c6f7564666c6172652e636f6d"

	tgLink := TelegramLink(host, port, secret)
	if !strings.HasPrefix(tgLink, "tg://proxy?") {
		t.Fatalf("expected tg://proxy? prefix, got %s", tgLink)
	}
	if !strings.Contains(tgLink, "server=vpn.example.com") || !strings.Contains(tgLink, "port=8443") {
		t.Fatalf("missing host/port in link: %s", tgLink)
	}

	httpsLink := TelegramHTTPSLink(host, port, secret)
	if !strings.HasPrefix(httpsLink, "https://t.me/proxy?") {
		t.Fatalf("expected https://t.me/proxy? prefix, got %s", httpsLink)
	}
}
