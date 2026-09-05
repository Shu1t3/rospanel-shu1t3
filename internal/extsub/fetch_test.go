package extsub

import (
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestSubscriptionHeaders(t *testing.T) {
	url1 := "https://panel.example.com/sub/token123"
	url2 := "https://panel.example.com/sub/token456"

	hdr1 := SubscriptionHeaders(url1)
	hdr1Repeat := SubscriptionHeaders(url1)
	hdr2 := SubscriptionHeaders(url2)

	hwid1 := hdr1.Get(model.HeaderHWID)
	hwid1Repeat := hdr1Repeat.Get(model.HeaderHWID)
	hwid2 := hdr2.Get(model.HeaderHWID)

	if hwid1 == "" {
		t.Fatal("expected non-empty x-hwid")
	}
	if hwid1 != hwid1Repeat {
		t.Fatalf("SubscriptionHeaders must be deterministic: %q != %q", hwid1, hwid1Repeat)
	}
	if hwid1 == hwid2 {
		t.Fatalf("different URLs must produce different x-hwid: %q == %q", hwid1, hwid2)
	}

	if got := hdr1.Get("User-Agent"); got != "RosPanel-ExtSub/1.0" {
		t.Errorf("User-Agent = %q, want RosPanel-ExtSub/1.0", got)
	}
	if got := hdr1.Get("Accept"); got != "text/plain, */*" {
		t.Errorf("Accept = %q, want text/plain, */*", got)
	}
	if got := hdr1.Get(model.HeaderDeviceOS); got != "RosPanel" {
		t.Errorf("x-device-os = %q, want RosPanel", got)
	}
	if got := hdr1.Get(model.HeaderOSVersion); got != "1.0" {
		t.Errorf("x-ver-os = %q, want 1.0", got)
	}
	if got := hdr1.Get(model.HeaderDeviceModel); got != "RosPanel Server" {
		t.Errorf("x-device-model = %q, want RosPanel Server", got)
	}
}
