package sub

import (
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func BenchmarkGenerateSingBox(b *testing.B) {
	set := &model.Settings{
		Host:         "example.com",
		VLESSPort:    443,
		HysteriaPort: 8443,
		SNI:          "example.com",
	}
	req := Request{
		User: model.User{
			ID:       1,
			Name:     "bench_user",
			UUID:     "11111111-2222-3333-4444-555555555555",
			Password: "secret_password",
		},
		Settings: set,
		Servers:  One(set),
		Access:   model.UnrestrictedAccess(),
	}

	b.ReportAllocs()
	for b.Loop() {
		res := GenerateSingBox(req)
		if len(res) == 0 {
			b.Fatal("empty singbox config")
		}
	}
}

func BenchmarkGenerateClash(b *testing.B) {
	set := &model.Settings{
		Host:         "example.com",
		VLESSPort:    443,
		HysteriaPort: 8443,
		SNI:          "example.com",
	}
	req := Request{
		User: model.User{
			ID:       1,
			Name:     "bench_user",
			UUID:     "11111111-2222-3333-4444-555555555555",
			Password: "secret_password",
		},
		Settings: set,
		Servers:  One(set),
		Access:   model.UnrestrictedAccess(),
	}

	b.ReportAllocs()
	for b.Loop() {
		res := GenerateClash(req)
		if len(res) == 0 {
			b.Fatal("empty clash config")
		}
	}
}
