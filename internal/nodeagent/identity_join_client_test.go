package nodeagent

import (
	"net/http"
	"testing"
)

// joinClient must carry a proper ALPN list so that CDN/proxy endpoints that
// respond with HTTP/2 (e.g. Cloudflare) are handled by the h2 transport rather
// than the HTTP/1.1 parser, which would die with "malformed HTTP response" on
// the binary SETTINGS frame.
func TestJoinClientTransportHasALPN(t *testing.T) {
	c := joinClient(false)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}
	if tr.TLSClientConfig == nil {
		// Clone of DefaultTransport: TLSClientConfig is nil, which means the
		// standard tls.Config is used — that already includes h2 in NextProtos
		// via the http2 package's ConfigureTransport path.  TLSNextProto must
		// NOT be an empty non-nil map (that would suppress h2).
		if tr.TLSNextProto != nil && len(tr.TLSNextProto) == 0 {
			t.Error("joinClient: TLSNextProto is an empty non-nil map — h2 is suppressed; CDN joins will fail with 'malformed HTTP response'")
		}
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("secure joinClient must not skip TLS verification")
	}
}

// The insecure variant should skip TLS verification but still allow h2 (unlike
// syncTransport, which explicitly forbids it).
func TestJoinClientInsecureAllowsH2(t *testing.T) {
	c := joinClient(true)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure joinClient must skip TLS verification")
	}
	// h2 must not be suppressed: TLSNextProto nil (default) is fine;
	// an empty non-nil map would suppress it.
	if tr.TLSNextProto != nil && len(tr.TLSNextProto) == 0 {
		t.Error("insecure joinClient: TLSNextProto is an empty non-nil map — h2 is suppressed")
	}
}
