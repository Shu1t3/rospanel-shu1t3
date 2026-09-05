package xray

import (
	"fmt"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func BenchmarkGenerateConfig(b *testing.B) {
	set := warpSettings()
	set.OperaEnabled = true
	set.RealityEnabled, set.RealityPrivateKey = true, "priv"
	set.RealityPublicKey, set.RealityShortID = "pub", "aabb"
	set.RealityDest = "www.microsoft.com:443"
	set.XrayDNS = "1.1.1.1\n8.8.8.8"

	// 100 users
	users := make([]model.User, 0, 100)
	for i := 1; i <= 100; i++ {
		users = append(users, model.User{
			ID:       int64(i),
			Name:     fmt.Sprintf("user_%d", i),
			UUID:     fmt.Sprintf("%08d-0000-0000-0000-000000000000", i),
			Password: fmt.Sprintf("pass_%d", i),
			Enabled:  true,
		})
	}
	opts := Options{
		PanelDest: "127.0.0.1:8080",
	}
	proxies := map[string][]model.ProxyEndpoint{}

	b.ReportAllocs()
	for b.Loop() {
		cfg, err := Generate(set, users, opts, proxies)
		if err != nil {
			b.Fatal(err)
		}
		if cfg == nil {
			b.Fatal("nil cfg")
		}
	}
}
