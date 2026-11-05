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

// The target IP must be assigned to the test host and outside the private
// ranges. Run in an isolated network so no public service is needed.
func TestMultiHopAgainstRealXray(t *testing.T) {
	bin, ip := os.Getenv("RP_XRAY_BIN"), os.Getenv("RP_MULTI_HOP_TEST_IP")
	if bin == "" || ip == "" {
		t.Skip("set RP_XRAY_BIN and RP_MULTI_HOP_TEST_IP")
	}

	target, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	origin := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "through the exit")
	})}
	go func() { _ = origin.Serve(target) }()
	t.Cleanup(func() { _ = origin.Close() })
	targetURL := "http://" + target.Addr().String()

	dir := t.TempDir()
	cert, key, pin := selfSignedCert(t, dir)
	entryPort, transitPort, exitPort, clientPort := freePort(t, "tcp"), freePort(t, "tcp"), freePort(t, "tcp"), freePort(t, "tcp")
	serviceLink := func(port int, id string) string {
		return fmt.Sprintf("vless://%s@127.0.0.1:%d?security=tls&sni=test&flow=%s&pcs=%s", id, port, VisionFlow, pin)
	}
	start := func(name string, port int, cfg any) func() {
		t.Helper()
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, name+".log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "run", "-c", path)
		cmd.Stdout, cmd.Stderr = f, f
		if err := cmd.Start(); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = f.Close()
			if t.Failed() {
				log, _ := os.ReadFile(f.Name())
				t.Logf("%s log: %s", name, log)
			}
		}
		t.Cleanup(stop)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
			if err == nil {
				_ = c.Close()
				return stop
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s did not start", name)
		return stop
	}
	generate := func(port int, users []model.User, opts Options) *Config {
		t.Helper()
		set := &model.Settings{Host: "test", SNI: "test", CertPath: cert, KeyPath: key, VLESSEnabled: port != entryPort, VLESSPort: port}
		if port == entryPort {
			set.VLESSPort = freePort(t, "tcp")
		}
		opts.PanelDest = "127.0.0.1:1"
		cfg, err := Generate(set, users, opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Log.Loglevel = "debug"
		for i := range cfg.Inbounds {
			if cfg.Inbounds[i].Tag == "api" {
				cfg.Inbounds[i].Port = freePort(t, "tcp")
			}
		}
		return cfg
	}
	in := model.Inbound{
		ID: 3, Name: "multi-hop", Port: entryPort, Protocol: model.InbVLESS, Enabled: true,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS, MultiHop: &model.MultiHop{Transit: 1, Exit: 2}},
	}
	in.Normalize()
	allowed := model.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}
	denied := model.User{ID: 2, UUID: "22222222-2222-4222-8222-222222222222"}
	transitID, exitID := "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"
	bridge := func(id, role string, hop MultiHopBridge) Bridge {
		return Bridge{Email: "bridge-multi-hop-3-" + role, UUID: id, Lane: model.BridgeVision, Sources: []string{"127.0.0.1"}, MultiHop: &hop}
	}
	exit := generate(exitPort, nil, Options{Bridges: []Bridge{bridge(exitID, "exit", MultiHopBridge{Exit: true})}})
	transit := generate(transitPort, nil, Options{Bridges: []Bridge{bridge(transitID, "transit", MultiHopBridge{Next: serviceLink(exitPort, exitID)})}})
	entry := generate(entryPort, []model.User{allowed, denied}, Options{
		Custom: []model.Inbound{in}, MultiHopLinks: map[int64]string{in.ID: serviceLink(transitPort, transitID)},
		Access: map[int64]model.Access{allowed.ID: {Tokens: map[string]bool{model.InboundToken(in.ID): true}}},
	})
	stopExit := start("exit", exitPort, exit)
	stopTransit := start("transit", transitPort, transit)
	start("entry", entryPort, entry)

	client := func(name string, port int, user model.User) {
		o, ok := linkOutbound(serviceLink(entryPort, in.VLESSUUID(user.UUID)), "entry", false)
		if !ok {
			t.Fatal("cannot generate client outbound")
		}
		start(name, port, map[string]any{
			"log":       map[string]string{"loglevel": "debug"},
			"inbounds":  []map[string]any{{"listen": "127.0.0.1", "port": port, "protocol": "http"}},
			"outbounds": []map[string]any{o},
		})
	}
	client("client", clientPort, allowed)
	deniedPort := freePort(t, "tcp")
	client("denied", deniedPort, denied)
	request := func(port int, want string) {
		t.Helper()
		proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		resp, err := (&http.Client{Transport: tr, Timeout: 4 * time.Second}).Get(targetURL)
		if err != nil {
			if want != "" {
				t.Fatal(err)
			}
			return
		}
		defer resp.Body.Close()
		if want == "" {
			if resp.StatusCode == http.StatusOK {
				t.Fatal("traffic leaked to the origin")
			}
			return
		}
		if data, err := io.ReadAll(resp.Body); err != nil || string(data) != want {
			t.Fatalf("origin response = %q, status = %v, err = %v", data, resp.Status, err)
		}
	}
	request(clientPort, "through the exit")
	request(deniedPort, "")
	stopExit()
	request(clientPort, "")
	start("exit-recovered", exitPort, exit)
	request(clientPort, "through the exit")
	stopTransit()
	request(clientPort, "")
	start("transit-recovered", transitPort, transit)
	request(clientPort, "through the exit")
}
