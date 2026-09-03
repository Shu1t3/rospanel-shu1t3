package happ

import (
	"encoding/base64"
	"testing"

	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

func TestToXrayOutboundVLESS(t *testing.T) {
	node := &Node{
		ID:       10,
		Protocol: "vless",
		Host:     "nl.example.com",
		Port:     443,
		URI:      "vless://11111111-2222-3333-4444-555555555555@nl.example.com:443?type=ws&security=tls&path=%2Fws&sni=nl.example.com#NL",
	}
	ob, err := ToXrayOutbound(node)
	if err != nil {
		t.Fatalf("ToXrayOutbound error: %v", err)
	}
	if ob.Tag != "happ-10" {
		t.Errorf("expected tag happ-10, got %q", ob.Tag)
	}
	if ob.Protocol != "vless" {
		t.Errorf("expected protocol vless, got %q", ob.Protocol)
	}
	ss, ok := ob.StreamSettings.(*xray.StreamSettings)
	if !ok || ss == nil || ss.Network != "ws" || ss.Security != "tls" {
		t.Errorf("streamSettings mismatch: %+v", ob.StreamSettings)
	}
}

func TestToXrayOutboundVMess(t *testing.T) {
	vmessURI := "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"add":"de.example.com","port":443,"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","net":"ws","tls":"tls","ps":"DE"}`))
	node := &Node{
		ID:       11,
		Protocol: "vmess",
		Host:     "de.example.com",
		Port:     443,
		URI:      vmessURI,
	}
	ob, err := ToXrayOutbound(node)
	if err != nil {
		t.Fatalf("ToXrayOutbound error: %v", err)
	}
	if ob.Tag != "happ-11" || ob.Protocol != "vmess" {
		t.Errorf("vmess outbound mismatch: %+v", ob)
	}
}

func TestToXrayOutboundTrojan(t *testing.T) {
	node := &Node{
		ID:       12,
		Protocol: "trojan",
		Host:     "fi.example.com",
		Port:     443,
		URI:      "trojan://trojanpass@fi.example.com:443?security=tls#FI",
	}
	ob, err := ToXrayOutbound(node)
	if err != nil {
		t.Fatalf("ToXrayOutbound error: %v", err)
	}
	if ob.Tag != "happ-12" || ob.Protocol != "trojan" {
		t.Errorf("trojan outbound mismatch: %+v", ob)
	}
}

func TestToXrayOutboundSS(t *testing.T) {
	node := &Node{
		ID:       13,
		Protocol: "ss",
		Host:     "se.example.com",
		Port:     8388,
		URI:      "ss://" + base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:sspass")) + "@se.example.com:8388#SE",
	}
	ob, err := ToXrayOutbound(node)
	if err != nil {
		t.Fatalf("ToXrayOutbound error: %v", err)
	}
	if ob.Tag != "happ-13" || ob.Protocol != "shadowsocks" {
		t.Errorf("ss outbound mismatch: %+v", ob)
	}
}

func TestToXrayOutboundHysteria2(t *testing.T) {
	node := &Node{
		ID:       14,
		Protocol: "hysteria2",
		Host:     "no.example.com",
		Port:     443,
		URI:      "hysteria2://hy2pass@no.example.com:443?sni=no.example.com#NO",
	}
	ob, err := ToXrayOutbound(node)
	if err != nil {
		t.Fatalf("ToXrayOutbound error: %v", err)
	}
	if ob.Tag != "happ-14" || ob.Protocol != "hysteria2" {
		t.Errorf("hy2 outbound mismatch: %+v", ob)
	}
}

func TestToXrayOutboundVLESSReality(t *testing.T) {
	node := &Node{
		ID:       15,
		Protocol: "vless",
		Host:     "nl.example.com",
		Port:     443,
		URI:      "vless://11111111-2222-3333-4444-555555555555@nl.example.com:443?type=tcp&security=reality&pbk=1234567890abcdef1234567890abcdef1234567890ab&sid=123456&spx=%2F&sni=yahoo.com&fp=chrome#NL-Reality",
	}
	ob, err := ToXrayOutbound(node)
	if err != nil {
		t.Fatalf("ToXrayOutbound error: %v", err)
	}
	if ob.Tag != "happ-15" {
		t.Errorf("expected tag happ-15, got %q", ob.Tag)
	}
	ss, ok := ob.StreamSettings.(*xray.StreamSettings)
	if !ok || ss == nil {
		t.Fatalf("expected *xray.StreamSettings, got %T", ob.StreamSettings)
	}
	if ss.Security != "reality" {
		t.Errorf("expected security reality, got %q", ss.Security)
	}
	if ss.TLSSettings != nil {
		t.Errorf("expected tlsSettings to be nil for reality, got %+v", ss.TLSSettings)
	}
	rs := ss.RealitySettings
	if rs == nil {
		t.Fatalf("expected realitySettings, got nil")
	}
	if rs.PublicKey != "1234567890abcdef1234567890abcdef1234567890ab" {
		t.Errorf("publicKey mismatch: got %q", rs.PublicKey)
	}
	if rs.ShortID != "123456" {
		t.Errorf("shortId mismatch: got %q", rs.ShortID)
	}
	if rs.SpiderX != "/" {
		t.Errorf("spiderX mismatch: got %q", rs.SpiderX)
	}
	if rs.ServerName != "yahoo.com" {
		t.Errorf("serverName mismatch: got %q", rs.ServerName)
	}
	if rs.Fingerprint != "chrome" {
		t.Errorf("fingerprint mismatch: got %q", rs.Fingerprint)
	}
}

func TestToXrayOutboundVLESSRealityMissingPublicKey(t *testing.T) {
	node := &Node{
		ID:       16,
		Protocol: "vless",
		Host:     "nl.example.com",
		Port:     443,
		URI:      "vless://11111111-2222-3333-4444-555555555555@nl.example.com:443?type=tcp&security=reality&sni=yahoo.com#NL-NoPK",
	}
	_, err := ToXrayOutbound(node)
	if err == nil {
		t.Fatalf("expected error for reality outbound without publicKey, got nil")
	}
}
