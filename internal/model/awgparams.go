package model

import (
	"encoding/json"
	"strconv"
	"strings"
)

// AWGParams are a server's AmneziaWG obfuscation parameters (AWG 3.1) as the
// store keeps them — the same fields as awg.Params, kept here so the model does
// not pull the tunnel engine into every package that reads a settings row.
type AWGParams struct {
	Jc   int `json:"jc"`
	Jmin int `json:"jmin"`
	Jmax int `json:"jmax"`
	S1   int `json:"s1"`
	S2   int `json:"s2"`
	S3   int `json:"s3,omitempty"`
	S4   int `json:"s4,omitempty"`

	// H1–H4 in AWG 3.1 can be a single number or a range "min-max" (e.g. "1000-5000").
	H1 string `json:"h1"`
	H2 string `json:"h2"`
	H3 string `json:"h3"`
	H4 string `json:"h4"`

	// I1–I5 are custom signature packets sent before handshakes (empty by default).
	I1 string `json:"i1,omitempty"`
	I2 string `json:"i2,omitempty"`
	I3 string `json:"i3,omitempty"`
	I4 string `json:"i4,omitempty"`
	I5 string `json:"i5,omitempty"`

	// HeaderProtectionKey is the ChaCha20 32-byte key (Base64).
	HeaderProtectionKey string `json:"header_protection_key,omitempty"`

	// ContentPaddingAddition is the range for transport padding (e.g. "0-32").
	ContentPaddingAddition string `json:"content_padding_addition,omitempty"`

	// RandomTrailers appends random bytes to packets.
	RandomTrailers bool `json:"random_trailers,omitempty"`

	// DisableCookies disables cookie reply under high load.
	DisableCookies bool `json:"disable_cookies,omitempty"`

	// Timing ranges (e.g. "110-130", "4-6").
	RekeyAfterTime       string `json:"rekey_after_time,omitempty"`
	RekeyTimeout         string `json:"rekey_timeout,omitempty"`
	RejectAfterTime      string `json:"reject_after_time,omitempty"`
	KeepaliveTimeout     string `json:"keepalive_timeout,omitempty"`
	MaxHandshakeAttempts string `json:"max_handshake_attempts,omitempty"`
}

// IsZero reports a parameter block that was never generated.
func (p AWGParams) IsZero() bool {
	return p.Jc == 0 && p.Jmin == 0 && p.Jmax == 0 && p.S1 == 0 && p.S2 == 0 &&
		p.H1 == "" && p.H2 == "" && p.H3 == "" && p.H4 == ""
}

// UnmarshalJSON handles both AWG 1.0 format (where H1..H4 were uint32 numbers)
// and AWG 3.1 format (where H1..H4 are strings or ranges).
func (p *AWGParams) UnmarshalJSON(data []byte) error {
	type rawAWGParams struct {
		Jc                     int    `json:"jc"`
		Jmin                   int    `json:"jmin"`
		Jmax                   int    `json:"jmax"`
		S1                     int    `json:"s1"`
		S2                     int    `json:"s2"`
		S3                     int    `json:"s3"`
		S4                     int    `json:"s4"`
		H1                     any    `json:"h1"`
		H2                     any    `json:"h2"`
		H3                     any    `json:"h3"`
		H4                     any    `json:"h4"`
		I1                     string `json:"i1"`
		I2                     string `json:"i2"`
		I3                     string `json:"i3"`
		I4                     string `json:"i4"`
		I5                     string `json:"i5"`
		HeaderProtectionKey    string `json:"header_protection_key"`
		ContentPaddingAddition string `json:"content_padding_addition"`
		RandomTrailers         any    `json:"random_trailers"`
		DisableCookies         any    `json:"disable_cookies"`
		RekeyAfterTime         string `json:"rekey_after_time"`
		RekeyTimeout           string `json:"rekey_timeout"`
		RejectAfterTime        string `json:"reject_after_time"`
		KeepaliveTimeout       string `json:"keepalive_timeout"`
		MaxHandshakeAttempts   string `json:"max_handshake_attempts"`
	}
	var raw rawAWGParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.Jc = raw.Jc
	p.Jmin = raw.Jmin
	p.Jmax = raw.Jmax
	p.S1 = raw.S1
	p.S2 = raw.S2
	p.S3 = raw.S3
	p.S4 = raw.S4
	p.H1 = parseHeaderField(raw.H1)
	p.H2 = parseHeaderField(raw.H2)
	p.H3 = parseHeaderField(raw.H3)
	p.H4 = parseHeaderField(raw.H4)
	p.I1 = raw.I1
	p.I2 = raw.I2
	p.I3 = raw.I3
	p.I4 = raw.I4
	p.I5 = raw.I5
	p.HeaderProtectionKey = raw.HeaderProtectionKey
	p.ContentPaddingAddition = raw.ContentPaddingAddition
	p.RandomTrailers = parseBoolField(raw.RandomTrailers)
	p.DisableCookies = parseBoolField(raw.DisableCookies)
	p.RekeyAfterTime = raw.RekeyAfterTime
	p.RekeyTimeout = raw.RekeyTimeout
	p.RejectAfterTime = raw.RejectAfterTime
	p.KeepaliveTimeout = raw.KeepaliveTimeout
	p.MaxHandshakeAttempts = raw.MaxHandshakeAttempts
	return nil
}

func parseHeaderField(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return strconv.FormatUint(uint64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	case json.Number:
		return val.String()
	default:
		return ""
	}
}

func parseBoolField(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lower := strings.ToLower(strings.TrimSpace(val))
		return lower == "true" || lower == "on" || lower == "1" || lower == "yes"
	case float64:
		return val != 0
	default:
		return false
	}
}
