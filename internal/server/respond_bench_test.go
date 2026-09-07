package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func BenchmarkWriteOK(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		writeOK(w)
	}
}

func BenchmarkDecodeJSONFastPath(b *testing.B) {
	payload := `{"name":"test","count":42}`
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest("POST", "/api/test", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		var dst struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		if !decodeJSON(w, req, &dst) {
			b.Fatal("decodeJSON failed")
		}
	}
}
