package mtproto

import (
	"context"
	"testing"

	"github.com/9seconds/mtg/v2/mtglib"
)

func BenchmarkGenerateSecret(b *testing.B) {
	for b.Loop() {
		_, err := GenerateSecret("cloudflare.com")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSecret(b *testing.B) {
	secHex, err := GenerateSecret("cloudflare.com")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := ParseSecret(secHex)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEventStreamZeroAlloc(b *testing.B) {
	m := NewMetrics()
	stream := NewEventStream(m)
	ctx := context.Background()
	evt := mtglib.NewEventTraffic("bench", 4096, true)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		stream.Send(ctx, evt)
	}
}
