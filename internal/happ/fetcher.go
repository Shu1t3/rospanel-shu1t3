package happ

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/netguard"
)

const (
	maxSubBodyBytes = 2 * 1024 * 1024 // 2 MB
	fetchTimeout    = 30 * time.Second
)

// FetchResult is the result of a single subscription fetch-and-parse.
type FetchResult struct {
	Nodes []Node
	Raw   []byte // raw body for diagnostics
}

// SubscriptionHeaders builds client headers including a deterministic x-hwid for
// fetching subscriptions from panels requiring device registration.
func SubscriptionHeaders(rawURL string) http.Header {
	h := sha256.Sum256([]byte("rospanel-happ:" + rawURL))
	hwid := fmt.Sprintf("rospanel-happ-%x", h[:16])

	hdr := make(http.Header)
	hdr.Set("User-Agent", "RosPanel-Happ/1.0")
	hdr.Set("Accept", "text/plain, */*")
	hdr.Set(model.HeaderHWID, hwid)
	hdr.Set(model.HeaderDeviceOS, "RosPanel")
	hdr.Set(model.HeaderOSVersion, "1.0")
	hdr.Set(model.HeaderDeviceModel, "Happ Subscription")
	return hdr
}

// Fetch downloads a subscription URL, decodes its body, and parses all proxy
// URIs into Node values. It uses the existing SSRF-safe netguard HTTP client
// (https-only, private IPs blocked, redirect-checked, size-limited).
func Fetch(ctx context.Context, rawURL string, subscriptionID int64) (*FetchResult, error) {
	if err := netguard.ValidateFetchURL(rawURL); err != nil {
		return nil, fmt.Errorf("subscription URL invalid: %w", err)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
	}

	body, err := netguard.GetWithHeaders(ctx, rawURL, maxSubBodyBytes, SubscriptionHeaders(rawURL))
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	lines := Decode(body)
	nodes := ParseURIs(lines, subscriptionID)

	return &FetchResult{
		Nodes: nodes,
		Raw:   body,
	}, nil
}
