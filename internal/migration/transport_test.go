package migration

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestCandidateURL(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"89.124.97.70", "https://89.124.97.70:8080"},
		{"89.124.97.70:8443", "https://89.124.97.70:8443"},
		{"https://candidate.example.com:8080", "https://candidate.example.com:8080"},
	} {
		got, err := CandidateURL(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("CandidateURL(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"http://89.124.97.70:8080", "https://candidate.example.com/path", "https://user@candidate.example.com"} {
		if _, err := CandidateURL(input); err == nil {
			t.Errorf("CandidateURL(%q) accepted unsafe address", input)
		}
	}
}

func TestCandidatePreflightUsesPinnedHTTPS(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef"
	cert, err := CandidateTLSCertificate(token)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/migration/health" {
			http.NotFound(w, r)
		}
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()

	client, err := CandidateClient(token, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(srv.URL + "/migration/health")
	if err != nil {
		t.Fatalf("pinned TLS request: %v", err)
	}
	_ = resp.Body.Close()

	badClient, err := CandidateClient("different-token", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := badClient.Get(srv.URL + "/migration/health"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong token accepted, err = %v", err)
	}

	checker := NewPreflightChecker()
	sess := &Session{PublicDomain: "vpn.example.com", CandidateAddr: srv.URL, PairToken: token}
	results, _ := checker.RunAllPreflightChecks(context.Background(), sess, &model.Settings{Host: "vpn.example.com"}, nil, nil)
	for _, result := range results {
		if result.Name == "candidate_reachability" && !result.Passed {
			t.Fatalf("candidate HTTPS preflight failed: %s", result.Error)
		}
	}
}
