package model

import (
	"encoding/json"
	"testing"
)

func TestAWGParamsUnmarshalLegacy(t *testing.T) {
	legacyJSON := `{"jc":5,"jmin":50,"jmax":1000,"s1":64,"s2":70,"h1":12345678,"h2":23456789,"h3":34567890,"h4":45678901}`
	var p AWGParams
	if err := json.Unmarshal([]byte(legacyJSON), &p); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if p.Jc != 5 || p.Jmin != 50 || p.Jmax != 1000 || p.S1 != 64 || p.S2 != 70 {
		t.Errorf("basic params mismatch: %+v", p)
	}
	if p.H1 != "12345678" || p.H2 != "23456789" || p.H3 != "34567890" || p.H4 != "45678901" {
		t.Errorf("header params mismatch: H1=%q H2=%q H3=%q H4=%q", p.H1, p.H2, p.H3, p.H4)
	}
}

func TestAWGParamsUnmarshalV3(t *testing.T) {
	v3JSON := `{
		"jc": 8, "jmin": 100, "jmax": 900, "s1": 117, "s2": 107, "s3": 81, "s4": 13,
		"h1": "332000358-332002244", "h2": "1931595695-1931642067", "h3": "2978398494-2978431738", "h4": "3980400275-3980438548",
		"i1": "<b 0x010203><r 10><t>",
		"header_protection_key": "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3RrZXk=",
		"content_padding_addition": "0-32",
		"random_trailers": true,
		"disable_cookies": "on",
		"rekey_after_time": "110-130"
	}`
	var p AWGParams
	if err := json.Unmarshal([]byte(v3JSON), &p); err != nil {
		t.Fatalf("unmarshal v3: %v", err)
	}
	if p.S3 != 81 || p.S4 != 13 || p.H1 != "332000358-332002244" {
		t.Errorf("v3 params mismatch: %+v", p)
	}
	if p.I1 != "<b 0x010203><r 10><t>" {
		t.Errorf("i1 mismatch: %q", p.I1)
	}
	if !p.RandomTrailers || !p.DisableCookies {
		t.Errorf("bool flags mismatch: random_trailers=%v disable_cookies=%v", p.RandomTrailers, p.DisableCookies)
	}
	if p.RekeyAfterTime != "110-130" {
		t.Errorf("rekey_after_time mismatch: %q", p.RekeyAfterTime)
	}
}
