package extsub

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/netguard"
)

const (
	maxBodyBytes = 2 << 20 // a subscription is a list of links; 2 MB is thousands of them
	fetchTimeout = 30 * time.Second
)

// IsURL reports whether a source is fetched (an http(s) address) rather than
// decoded in place (a pasted happ:// link, a base64 blob, a list of links).
func IsURL(source string) bool {
	s := strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// SubscriptionHeaders constructs client and device headers for fetching an
// external subscription. It provides a deterministic x-hwid derived from the
// source URL so that upstream panels requiring device registration (HWIDRequire)
// admit the request and bind the same device slot across hourly syncs.
func SubscriptionHeaders(source string) http.Header {
	h := sha256.Sum256([]byte("rospanel-extsub:" + source))
	hwid := fmt.Sprintf("rospanel-%x", h[:16])

	hdr := make(http.Header)
	hdr.Set("User-Agent", "RosPanel-ExtSub/1.0")
	hdr.Set("Accept", "text/plain, */*")
	hdr.Set(model.HeaderHWID, hwid)
	hdr.Set(model.HeaderDeviceOS, "RosPanel")
	hdr.Set(model.HeaderOSVersion, "1.0")
	hdr.Set(model.HeaderDeviceModel, "RosPanel Server")
	return hdr
}

// ValidateSource checks a source before it is stored: a URL must pass the same
// SSRF gate as every other address the panel fetches, and an inline payload must
// decode to at least one usable link — an operator pasting the wrong thing is
// told now, not by an empty list later.
func ValidateSource(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return errors.New("empty source")
	}
	if IsURL(source) {
		return netguard.ValidateFetchURL(source)
	}
	if len(source) > maxBodyBytes {
		return errors.New("payload too large")
	}
	if len(ParseAll(Decode([]byte(source)))) == 0 {
		return errors.New("no share links found in the payload")
	}
	return nil
}

// Load resolves a source to its endpoints: fetched through the SSRF-safe client
// when it is a URL, decoded in place otherwise.
func Load(ctx context.Context, source string) ([]Endpoint, error) {
	source = strings.TrimSpace(source)
	var body []byte
	if IsURL(source) {
		if err := netguard.ValidateFetchURL(source); err != nil {
			return nil, err
		}
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, fetchTimeout)
			defer cancel()
		}
		var err error
		if body, err = netguard.GetWithHeaders(ctx, source, maxBodyBytes, SubscriptionHeaders(source)); err != nil {
			return nil, fmt.Errorf("fetch: %w", err)
		}
	} else {
		body = []byte(source)
	}
	eps := ParseAll(Decode(body))
	if len(eps) == 0 {
		return nil, errors.New("no share links found")
	}
	return eps, nil
}
