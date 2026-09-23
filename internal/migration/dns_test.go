package migration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestCloudflareAdapter(t *testing.T) {
	// Mock Cloudflare API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/client/v4/zones" {
			_, _ = w.Write([]byte(`{
				"success": true,
				"result": [{"id": "mock-zone-123", "name": "example.com"}]
			}`))
			return
		}
		if r.URL.Path == "/client/v4/zones/mock-zone-123/dns_records" {
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{
					"success": true,
					"result": [
						{"id": "rec-1", "type": "A", "name": "vpn.example.com", "content": "1.1.1.1", "ttl": 300}
					]
				}`))
				return
			}
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"success": true, "result": {"id": "rec-created"}}`))
				return
			}
		}
		if r.Method == http.MethodPatch {
			_, _ = w.Write([]byte(`{"success": true, "result": {"id": "rec-updated"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Direct client to mock server
	adapter := &CloudflareAdapter{
		client: server.Client(),
		config: CloudflareConfig{
			APIToken: "mock-token",
			ZoneID:   "mock-zone-123",
		},
	}

	if adapter.Provider() != "cloudflare" {
		t.Errorf("expected provider cloudflare, got %s", adapter.Provider())
	}
}

func TestManualAdapter(t *testing.T) {
	adapter := NewManualAdapter()
	if adapter.Provider() != "manual" {
		t.Errorf("expected provider manual, got %s", adapter.Provider())
	}

	res, err := adapter.SwitchRecord(context.Background(), "vpn.example.com", "192.0.2.1")
	if err != nil {
		t.Fatalf("SwitchRecord: %v", err)
	}
	if !res.RequiresWait || len(res.NewIPs) == 0 || res.NewIPs[0] != "192.0.2.1" {
		t.Errorf("unexpected manual switch result: %+v", res)
	}
}

func TestPreflightChecks(t *testing.T) {
	pc := NewPreflightChecker()

	sess := &Session{
		PublicDomain:  "vpn.example.com",
		CandidateAddr: "192.0.2.100:8080",
	}
	set := &model.Settings{
		Host:            "vpn.example.com",
		VLESSEnabled:    true,
		RealityEnabled:  true,
		HysteriaEnabled: true,
		CertPath:        "/path/to/cert.pem",
	}

	users := []model.User{
		{ID: 1, UUID: "user-uuid-123"},
	}

	results, ok := pc.RunAllPreflightChecks(context.Background(), sess, set, users, nil)
	// Candidate addr 192.0.2.100:8080 won't be reachable in mock, so ok will be false, but checks run
	if len(results) == 0 {
		t.Fatal("expected preflight checks results")
	}

	foundDomainCheck := false
	foundSubCheck := false
	for _, r := range results {
		if r.Name == "domain_isolation" {
			foundDomainCheck = true
			if !r.Passed {
				t.Errorf("expected domain_isolation to pass, got err: %s", r.Error)
			}
		}
		if r.Name == "subscription_parity" {
			foundSubCheck = true
			if !r.Passed {
				t.Errorf("expected subscription_parity to pass, got err: %s", r.Error)
			}
		}
	}
	if !foundDomainCheck || !foundSubCheck {
		t.Errorf("missing critical checks in results: %+v", results)
	}
	_ = ok
}

func TestDomainIsolationFailureWhenEqual(t *testing.T) {
	pc := NewPreflightChecker()

	// Violation: CandidateAddr is same as PublicDomain
	sess := &Session{
		PublicDomain:  "vpn.example.com",
		CandidateAddr: "vpn.example.com",
	}
	set := &model.Settings{
		Host: "vpn.example.com",
	}

	results, ok := pc.RunAllPreflightChecks(context.Background(), sess, set, nil, nil)
	if ok {
		t.Error("expected preflight to fail when candidate addr equals public domain")
	}
	for _, r := range results {
		if r.Name == "domain_isolation" && r.Passed {
			t.Error("domain_isolation check should fail when candidate addr equals public domain")
		}
	}
}
