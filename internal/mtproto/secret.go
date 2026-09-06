package mtproto

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/9seconds/mtg/v2/mtglib"
)

// GenerateSecret creates a new FakeTLS secret for the specified SNI domain.
// If domain is empty, a fallback domain "cloudflare.com" is used.
func GenerateSecret(domain string) (string, error) {
	d := strings.TrimSpace(domain)
	if d == "" {
		d = "cloudflare.com"
	}
	sec := mtglib.GenerateSecret(d)
	if !sec.Valid() {
		return "", errors.New("failed to generate valid FakeTLS secret")
	}
	return sec.Hex(), nil
}

// ParseSecret parses a raw secret string (hex or base64) into an mtglib.Secret.
func ParseSecret(secretStr string) (mtglib.Secret, error) {
	s := strings.TrimSpace(secretStr)
	if s == "" {
		return mtglib.Secret{}, errors.New("secret cannot be empty")
	}
	sec, err := mtglib.ParseSecret(s)
	if err != nil {
		return mtglib.Secret{}, fmt.Errorf("parse mtproto secret: %w", err)
	}
	if !sec.Valid() {
		return mtglib.Secret{}, errors.New("invalid mtproto secret format")
	}
	return sec, nil
}

// TelegramLink builds the tg:// scheme proxy connection link.
func TelegramLink(host string, port int, secretStr string) string {
	q := url.Values{}
	q.Set("server", host)
	q.Set("port", fmt.Sprintf("%d", port))
	q.Set("secret", strings.TrimSpace(secretStr))
	return "tg://proxy?" + q.Encode()
}

// TelegramHTTPSLink builds the https://t.me/proxy connection link.
func TelegramHTTPSLink(host string, port int, secretStr string) string {
	q := url.Values{}
	q.Set("server", host)
	q.Set("port", fmt.Sprintf("%d", port))
	q.Set("secret", strings.TrimSpace(secretStr))
	return "https://t.me/proxy?" + q.Encode()
}
