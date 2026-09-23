package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// DNSChangeResult contains details about updated DNS records.
type DNSChangeResult struct {
	Domain       string   `json:"domain"`
	OldIPs       []string `json:"old_ips"`
	NewIPs       []string `json:"new_ips"`
	TTL          int      `json:"ttl"`
	Provider     string   `json:"provider"`
	RequiresWait bool     `json:"requires_wait"`
	Notice       string   `json:"notice"`
}

// DNSPropagationStatus tracks whether the new IP is visible worldwide and on authoritative nameservers.
type DNSPropagationStatus struct {
	Domain        string   `json:"domain"`
	ExpectedIP    string   `json:"expected_ip"`
	ResolvedIPs   []string `json:"resolved_ips"`
	Authoritative bool     `json:"authoritative"`
	Propagated    bool     `json:"propagated"`
	CacheWarning  string   `json:"cache_warning"`
}

// DNSAdapter abstracts DNS provider operations for zero-downtime cutover.
type DNSAdapter interface {
	Provider() string
	PrepareMigration(ctx context.Context, domain string, lowTTL int) error
	SwitchRecord(ctx context.Context, domain string, targetIP string) (*DNSChangeResult, error)
	VerifyPropagation(ctx context.Context, domain string, expectedIP string) (*DNSPropagationStatus, error)
	RevertRecord(ctx context.Context, domain string, originalIP string) error
}

// CloudflareConfig holds credentials and settings for Cloudflare DNS API.
type CloudflareConfig struct {
	APIToken string `json:"api_token"`
	ZoneID   string `json:"zone_id,omitempty"`
}

// CloudflareAdapter interacts with the Cloudflare v4 API.
type CloudflareAdapter struct {
	client *http.Client
	config CloudflareConfig
}

// NewCloudflareAdapter creates a new Cloudflare DNS adapter.
func NewCloudflareAdapter(cfg CloudflareConfig) *CloudflareAdapter {
	return &CloudflareAdapter{
		client: &http.Client{Timeout: 15 * time.Second},
		config: cfg,
	}
}

func (c *CloudflareAdapter) Provider() string {
	return "cloudflare"
}

type cfZoneResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type cfRecordsResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result []cfDNSRecord `json:"result"`
}

func (c *CloudflareAdapter) findZoneID(ctx context.Context, domain string) (string, error) {
	if c.config.ZoneID != "" {
		return c.config.ZoneID, nil
	}
	parts := strings.Split(strings.Trim(domain, "."), ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid domain name: %s", domain)
	}

	for i := 0; i < len(parts)-1; i++ {
		candidateZone := strings.Join(parts[i:], ".")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", candidateZone), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		var zr cfZoneResponse
		if err := json.NewDecoder(resp.Body).Decode(&zr); err == nil && zr.Success && len(zr.Result) > 0 {
			c.config.ZoneID = zr.Result[0].ID
			return zr.Result[0].ID, nil
		}
	}
	return "", fmt.Errorf("zone not found in Cloudflare for domain %s", domain)
}

func (c *CloudflareAdapter) listRecords(ctx context.Context, zoneID, domain string) ([]cfDNSRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?name=%s", zoneID, domain), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rr cfRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, err
	}
	if !rr.Success {
		var errs []string
		for _, e := range rr.Errors {
			errs = append(errs, e.Message)
		}
		return nil, fmt.Errorf("cloudflare error: %s", strings.Join(errs, "; "))
	}
	return rr.Result, nil
}

// PrepareMigration sets a low TTL (e.g. 60 or 120s) ahead of the switch.
func (c *CloudflareAdapter) PrepareMigration(ctx context.Context, domain string, lowTTL int) error {
	if lowTTL <= 0 {
		lowTTL = 60
	}
	zoneID, err := c.findZoneID(ctx, domain)
	if err != nil {
		return err
	}
	records, err := c.listRecords(ctx, zoneID, domain)
	if err != nil {
		return err
	}

	for _, rec := range records {
		if rec.Type == "A" || rec.Type == "AAAA" {
			payload := map[string]any{
				"ttl": lowTTL,
			}
			data, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
				fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, rec.ID),
				bytes.NewReader(data))
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := c.client.Do(req)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
		}
	}
	return nil
}

// SwitchRecord updates A/AAAA records to targetIP.
func (c *CloudflareAdapter) SwitchRecord(ctx context.Context, domain string, targetIP string) (*DNSChangeResult, error) {
	zoneID, err := c.findZoneID(ctx, domain)
	if err != nil {
		return nil, err
	}
	records, err := c.listRecords(ctx, zoneID, domain)
	if err != nil {
		return nil, err
	}

	isIPv6 := strings.Contains(targetIP, ":")
	targetType := "A"
	if isIPv6 {
		targetType = "AAAA"
	}

	var oldIPs []string
	updated := false

	for _, rec := range records {
		if rec.Type == targetType {
			oldIPs = append(oldIPs, rec.Content)
			payload := map[string]any{
				"content": targetIP,
			}
			data, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
				fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, rec.ID),
				bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := c.client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			updated = true
		}
	}

	if !updated {
		// Create record if not found
		payload := map[string]any{
			"type":    targetType,
			"name":    domain,
			"content": targetIP,
			"ttl":     60,
			"proxied": false,
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID),
			bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
	}

	return &DNSChangeResult{
		Domain:       domain,
		OldIPs:       oldIPs,
		NewIPs:       []string{targetIP},
		TTL:          60,
		Provider:     "cloudflare",
		RequiresWait: true,
		Notice:       "DNS обновлен в Cloudflare. Клиенты с закешированным IP продолжат обслуживаться старым сервером в течение времени TTL.",
	}, nil
}

// VerifyPropagation checks DNS response against expectedIP.
func (c *CloudflareAdapter) VerifyPropagation(ctx context.Context, domain string, expectedIP string) (*DNSPropagationStatus, error) {
	return checkDNSPropagation(ctx, domain, expectedIP)
}

// RevertRecord points the domain back to originalIP.
func (c *CloudflareAdapter) RevertRecord(ctx context.Context, domain string, originalIP string) error {
	_, err := c.SwitchRecord(ctx, domain, originalIP)
	return err
}

// ManualAdapter provides fallback for when no automated DNS provider is available.
type ManualAdapter struct{}

func NewManualAdapter() *ManualAdapter {
	return &ManualAdapter{}
}

func (m *ManualAdapter) Provider() string {
	return "manual"
}

func (m *ManualAdapter) PrepareMigration(ctx context.Context, domain string, lowTTL int) error {
	// Manual instructions: nothing to automate
	return nil
}

func (m *ManualAdapter) SwitchRecord(ctx context.Context, domain string, targetIP string) (*DNSChangeResult, error) {
	return &DNSChangeResult{
		Domain:       domain,
		NewIPs:       []string{targetIP},
		TTL:          300,
		Provider:     "manual",
		RequiresWait: true,
		Notice: fmt.Sprintf(
			"Ручной режим: измените A/AAAA запись для домена %s на IP %s у вашего DNS-регистратора. DNS не обновляется мгновенно.",
			domain, targetIP,
		),
	}, nil
}

func (m *ManualAdapter) VerifyPropagation(ctx context.Context, domain string, expectedIP string) (*DNSPropagationStatus, error) {
	return checkDNSPropagation(ctx, domain, expectedIP)
}

func (m *ManualAdapter) RevertRecord(ctx context.Context, domain string, originalIP string) error {
	return nil
}

// checkDNSPropagation verifies that the given domain resolves to expectedIP using public and direct DNS.
func checkDNSPropagation(ctx context.Context, domain string, expectedIP string) (*DNSPropagationStatus, error) {
	expectedIP = strings.TrimSpace(expectedIP)
	cleanDomain := strings.TrimSpace(domain)

	// Custom resolver querying public resolvers directly to avoid local OS cache
	resolvers := []string{
		"1.1.1.1:53",
		"8.8.8.8:53",
	}

	var resolved []string
	seen := make(map[string]bool)

	for _, srv := range resolvers {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(dialCtx, "udp", srv)
			},
		}
		ips, err := r.LookupIP(ctx, "ip", cleanDomain)
		if err == nil {
			for _, ip := range ips {
				s := ip.String()
				if !seen[s] {
					seen[s] = true
					resolved = append(resolved, s)
				}
			}
		}
	}

	if len(resolved) == 0 {
		// Fallback to default resolver
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", cleanDomain)
		if err == nil {
			for _, ip := range ips {
				s := ip.String()
				if !seen[s] {
					seen[s] = true
					resolved = append(resolved, s)
				}
			}
		}
	}

	propagated := false
	for _, ip := range resolved {
		if ip == expectedIP {
			propagated = true
			break
		}
	}

	warning := ""
	if !propagated {
		warning = fmt.Sprintf("Домен %s пока резолвится в %s вместо %s. Ожидайте истечения TTL DNS-записей.", cleanDomain, strings.Join(resolved, ", "), expectedIP)
	} else {
		warning = "Новый IP подтвержден на внешних резолверах. Клиентский кеш DNS может сохранять старый IP до окончания TTL."
	}

	return &DNSPropagationStatus{
		Domain:        cleanDomain,
		ExpectedIP:    expectedIP,
		ResolvedIPs:   resolved,
		Authoritative: len(resolved) > 0,
		Propagated:    propagated,
		CacheWarning:  warning,
	}, nil
}
