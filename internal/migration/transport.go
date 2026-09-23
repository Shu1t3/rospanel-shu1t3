package migration

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CandidateURL is the one address used for installation, preflight and transfer.
// A bare host defaults to the candidate's HTTPS port, never to public :443.
func CandidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("укажите технический адрес кандидата")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("адрес кандидата должен быть IP или доменом с необязательным портом HTTPS")
	}
	port := u.Port()
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("порт кандидата должен быть от 1 до 65535")
	}
	return "https://" + net.JoinHostPort(u.Hostname(), port), nil
}

// CandidateTLSCertificate creates a self-signed transport certificate whose key
// is bound to the high-entropy pairing token. The master pins that public key;
// no CA or public-domain DNS change is needed before cutover.
func CandidateTLSCertificate(token string) (tls.Certificate, error) {
	if token == "" {
		return tls.Certificate{}, errors.New("missing candidate pairing token")
	}
	seed := sha256.Sum256([]byte("rospanel candidate TLS v1:" + token))
	key := ed25519.NewKeyFromSeed(seed[:])
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "RosPanel migration candidate"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
}

// CandidateClient verifies the candidate's public key derived from the pairing
// token. Chain verification is intentionally replaced with this stronger pin for
// the candidate's self-signed technical-address certificate.
func CandidateClient(token string, timeout time.Duration) (*http.Client, error) {
	if token == "" {
		return nil, errors.New("missing candidate pairing token")
	}
	seed := sha256.Sum256([]byte("rospanel candidate TLS v1:" + token))
	want := ed25519.NewKeyFromSeed(seed[:]).Public().(ed25519.PublicKey)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true, // VerifyPeerCertificate pins the expected key below.
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("candidate did not present a certificate")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			got, ok := cert.PublicKey.(ed25519.PublicKey)
			if !ok || subtle.ConstantTimeCompare(got, want) != 1 {
				return errors.New("candidate certificate does not match the pairing token")
			}
			if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
				return fmt.Errorf("candidate certificate is outside its validity period")
			}
			return nil
		},
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // never send a snapshot or token to a redirect target
		},
	}, nil
}
