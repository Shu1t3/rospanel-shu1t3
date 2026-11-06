package xray

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestReverseMultiHopAgainstRealXray(t *testing.T) {
	bin, ip := os.Getenv("RP_XRAY_BIN"), os.Getenv("RP_MULTI_HOP_TEST_IP")
	if bin == "" || ip == "" {
		t.Skip("set RP_XRAY_BIN and RP_MULTI_HOP_TEST_IP")
	}
	target, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	origin := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "reverse-chain origin")
	})}
	go func() { _ = origin.Serve(target) }()
	t.Cleanup(func() { _ = origin.Close() })

	dir := t.TempDir()
	cert, key, pin := selfSignedCert(t, dir)
	start := func(name string, cfg any) func() {
		t.Helper()
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(bin, "run", "-test", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("Xray rejected %s: %v\n%s", name, err, output)
		}
		log, err := os.Create(filepath.Join(dir, name+".log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "run", "-c", path)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			_ = log.Close()
			t.Fatal(err)
		}
		stop := func() {
			if cmd.ProcessState != nil {
				return
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = log.Close()
		}
		t.Cleanup(func() {
			stop()
			if t.Failed() {
				body, _ := os.ReadFile(log.Name())
				t.Logf("%s: %s", name, body)
			}
		})
		return stop
	}
	const tag = "protected-hop-test"
	const (
		entryR = "11111111-1111-4111-8111-111111111111"
		exitR  = "22222222-2222-4222-8222-222222222222"
		entryB = "33333333-3333-4333-8333-333333333333"
		exitB  = "44444444-4444-4444-8444-444444444444"
	)
	entryPort, relayPort, transitPort, clientPort := freePort(t, "tcp"), freePort(t, "tcp"), freePort(t, "tcp"), freePort(t, "tcp")
	peer := func(port int, uuid string) ReverseChainPeer {
		return ReverseChainPeer{Address: "127.0.0.1", Port: port, SNI: "test", Pin: pin, UUID: uuid}
	}
	via := []int64{1, 2, 3}
	entry := model.Inbound{ID: 1, Port: entryPort, Enabled: true, Protocol: model.InbVLESS,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS, MultiHop: &model.MultiHop{Via: &via}}}
	user := model.User{ID: 1, UUID: "55555555-5555-4555-8555-555555555555", Enabled: true}
	generate := func(chain ReverseChain, isEntry bool) *Config {
		t.Helper()
		set := &model.Settings{Host: "test", SNI: "test", CertPath: cert, KeyPath: key, VLESSPort: freePort(t, "tcp")}
		opts := Options{PanelDest: "127.0.0.1:1", ReverseChains: []ReverseChain{chain}}
		if isEntry {
			opts.Custom = []model.Inbound{entry}
			opts.Access = map[int64]model.Access{user.ID: {Tokens: map[string]bool{model.InboundToken(entry.ID): true}}}
		}
		cfg, err := Generate(set, []model.User{user}, opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := range cfg.Inbounds {
			if cfg.Inbounds[i].Tag == "api" {
				cfg.Inbounds[i].Port = freePort(t, "tcp")
			}
		}
		cfg.Log.Loglevel = "info"
		return cfg
	}
	a := generate(ReverseChain{Tag: tag, Entry: &ReverseChainEntry{Inbound: entry.Tag(), Relay: peer(relayPort, entryR), Transit: peer(transitPort, entryB)}}, true)
	r := generate(ReverseChain{Tag: tag, Relay: &ReverseChainRelay{Port: relayPort, EntryUUID: entryR, ExitUUID: exitR, TransitIP: "127.0.0.1", TransitPort: transitPort}}, false)
	b := generate(ReverseChain{Tag: tag, Transit: &ReverseChainTransit{Port: transitPort, TrafficUUID: entryB, TunnelUUID: exitB, Sources: []string{"127.0.0.1"}}}, false)
	c := generate(ReverseChain{Tag: tag, Exit: &ReverseChainExit{Relay: peer(relayPort, exitR), Tunnel: peer(transitPort, exitB)}}, false)
	start("entry", a)
	stopRelay := start("relay", r)
	start("transit", b)
	client := map[string]any{
		"inbounds":  []map[string]any{{"listen": "127.0.0.1", "port": clientPort, "protocol": "http"}},
		"outbounds": []Outbound{reverseChainOutbound("client", peer(entryPort, entry.VLESSUUID(user.UUID)), "", "")},
	}
	start("client", client)
	proxy, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", clientPort))
	if err != nil {
		t.Fatal(err)
	}
	request := func() bool {
		tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		resp, err := (&http.Client{Transport: tr, Timeout: 2 * time.Second}).Get("http://" + target.Addr().String())
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode == http.StatusOK && string(body) == "reverse-chain origin"
	}
	check := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for {
			if request() == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("request success = %t, want %t", !want, want)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	check(false)
	stopExit := start("exit", c)
	check(true)
	stopExit()
	check(false)
	start("exit-recovered", c)
	check(true)
	stopRelay()
	check(false)
	start("relay-recovered", r)
	check(true)
}
