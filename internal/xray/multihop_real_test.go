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
	for _, length := range []int{1, 2, 3, 5, 12} {
		t.Run(fmt.Sprintf("%d-nodes", length), func(t *testing.T) {
			testMultiHopAgainstRealXray(t, bin, ip, length)
		})
	}
}

func testMultiHopAgainstRealXray(t *testing.T, bin, ip string, length int) {
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

	udp, err := net.ListenPacket("udp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := udp.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udp.WriteTo(buf[:n], addr)
		}
	}()

	dir := t.TempDir()
	cert, key, pin := selfSignedCert(t, dir)
	ports := make([]int, length)
	for i := range ports {
		ports[i] = freePort(t, "tcp")
	}
	entryPort, clientPort := ports[0], freePort(t, "tcp")
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
	via := make([]int64, length-1)
	for i := range via {
		via[i] = int64(i + 1)
	}
	in := model.Inbound{
		ID: 3, Name: "multi-hop", Port: entryPort, Protocol: model.InbVLESS, Enabled: true,
		Opts: model.InboundOpts{Transport: model.TrTCP, Security: model.SecTLS, MultiHop: &model.MultiHop{Via: &via}},
	}
	in.Normalize()
	allowed := model.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}
	denied := model.User{ID: 2, UUID: "22222222-2222-4222-8222-222222222222"}
	serviceID := func(i int) string { return fmt.Sprintf("33333333-3333-4333-8333-%012d", i) }
	configs := make([]*Config, length)
	stops := make([]func(), length)
	for i := length - 1; i > 0; i-- {
		hop := MultiHopBridge{Exit: i == length-1}
		if !hop.Exit {
			hop.Next = serviceLink(ports[i+1], serviceID(i+1))
		}
		bridge := Bridge{
			Email: fmt.Sprintf("bridge-multi-hop-3-%d", i), UUID: serviceID(i),
			Lane: model.BridgeVision, Sources: []string{"127.0.0.1"}, MultiHop: &hop,
		}
		configs[i] = generate(ports[i], nil, Options{Bridges: []Bridge{bridge}})
		stops[i] = start(fmt.Sprintf("hop-%d", i), ports[i], configs[i])
	}
	next := MultiHopLocalExit
	if length > 1 {
		next = serviceLink(ports[1], serviceID(1))
	}
	entry := generate(entryPort, []model.User{allowed, denied}, Options{
		Custom: []model.Inbound{in}, MultiHopLinks: map[int64]string{in.ID: next},
		Access: map[int64]model.Access{allowed.ID: {Tokens: map[string]bool{model.InboundToken(in.ID): true}}},
	})
	start("entry", entryPort, entry)

	client := func(name string, port, udpPort int, user model.User) {
		o, ok := linkOutbound(serviceLink(entryPort, in.VLESSUUID(user.UUID)), "entry", false)
		if !ok {
			t.Fatal("cannot generate client outbound")
		}
		inbounds := []map[string]any{{"listen": "127.0.0.1", "port": port, "protocol": "http"}}
		if udpPort != 0 {
			inbounds = append(inbounds, map[string]any{
				"listen": "127.0.0.1", "port": udpPort, "protocol": "dokodemo-door",
				"settings": map[string]any{"address": ip, "port": udp.LocalAddr().(*net.UDPAddr).Port, "network": "udp"},
			})
		}
		start(name, port, map[string]any{
			"log": map[string]string{"loglevel": "debug"}, "inbounds": inbounds, "outbounds": []map[string]any{o},
		})
	}
	udpPort := freePort(t, "udp")
	client("client", clientPort, udpPort, allowed)
	deniedPort := freePort(t, "tcp")
	client("denied", deniedPort, 0, denied)
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
	requestUDP := func(want bool) {
		t.Helper()
		c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", udpPort))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		timeout := time.Second
		if want {
			timeout = max(4*time.Second, time.Duration(length)*500*time.Millisecond)
		}
		_ = c.SetDeadline(time.Now().Add(timeout))
		if _, err := c.Write([]byte("through the exit")); err != nil {
			t.Fatal(err)
		}
		var buf [64]byte
		n, err := c.Read(buf[:])
		if want && (err != nil || string(buf[:n]) != "through the exit") {
			t.Fatalf("UDP through chain: %q (%v)", buf[:n], err)
		}
		if !want && err == nil {
			t.Fatal("UDP leaked to the origin")
		}
	}
	request(clientPort, "through the exit")
	requestUDP(true)
	request(deniedPort, "")
	// For the longer smoke case, interrupt one representative middle hop.
	first, last := 1, length
	if length > 5 {
		first, last = length/2, length/2+1
	}
	for i := first; i < last; i++ {
		stops[i]()
		request(clientPort, "")
		requestUDP(false)
		start(fmt.Sprintf("hop-%d-recovered", i), ports[i], configs[i])
		request(clientPort, "through the exit")
		requestUDP(true)
	}
}
