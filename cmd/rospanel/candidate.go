package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/backup"
	"github.com/Shu1t3/rospanel-shu1t3/internal/migration"
	"github.com/Shu1t3/rospanel-shu1t3/internal/version"
)

const candidateUnitPath = "/etc/systemd/system/rospanel.service"
const candidateDefaultData = "/var/lib/rospanel-candidate"

type candidateConfig struct {
	Address   string `json:"address"`
	MasterURL string `json:"master_url"`
	PairToken string `json:"pair_token"`
}

func candidateConfigPath(dataDir string) string { return filepath.Join(dataDir, "candidate.json") }

// runCandidateInstall makes the candidate survive the install script and SSH logout.
func runCandidateInstall(dataDir string, args []string) {
	if os.Geteuid() != 0 {
		log.Fatal("candidate install: run as root")
	}
	if os.Getenv("ROSPANEL_DATA") == "" {
		dataDir = candidateDefaultData
	}
	if existing, err := os.ReadFile(candidateUnitPath); err == nil && !strings.Contains(string(existing), "RosPanel master migration candidate") {
		log.Fatal("candidate install: this server already has a RosPanel master service")
	}
	cfg := parseCandidateConfig(args)
	if cfg.MasterURL == "" || cfg.PairToken == "" {
		log.Fatal("candidate install: --master and --pair-token are required")
	}
	endpoint, err := migration.CandidateURL(cfg.Address)
	if err != nil {
		log.Fatalf("candidate install: %v", err)
	}
	cfg.Address = endpoint
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("candidate install: create data dir: %v", err)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		log.Fatalf("candidate install: config: %v", err)
	}
	if err := os.WriteFile(candidateConfigPath(dataDir), b, 0o600); err != nil {
		log.Fatalf("candidate install: save config: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		log.Fatalf("candidate install: executable: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	if self != installBinPath {
		if err := copyFile(self, installBinPath, 0o755); err != nil {
			log.Fatalf("candidate install: install binary: %v", err)
		}
	}
	unit := "[Unit]\nDescription=RosPanel master migration candidate\nAfter=network-online.target\nWants=network-online.target\n\n" +
		"[Service]\nType=simple\nEnvironment=ROSPANEL_DATA=" + dataDir + "\nExecStart=" + installBinPath + " candidate\n" +
		"Restart=always\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n"
	if err := os.WriteFile(candidateUnitPath, []byte(unit), 0o644); err != nil {
		log.Fatalf("candidate install: write service: %v", err)
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "rospanel"}, {"restart", "rospanel"}} {
		cmd := exec.Command("systemctl", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("candidate install: systemctl %s: %v", strings.Join(args, " "), err)
		}
	}
	client, err := migration.CandidateClient(cfg.PairToken, 2*time.Second)
	if err != nil {
		log.Fatalf("candidate install: TLS client: %v", err)
	}
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(endpoint, "https://"))
	healthURL := "https://127.0.0.1:" + port + "/migration/health"
	ready := false
	for i := 0; i < 20; i++ {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !ready {
		log.Fatal("candidate install: HTTPS service did not become ready; check journalctl -u rospanel")
	}
	log.Printf("candidate: HTTPS service started on %s", endpoint)
}

func parseCandidateConfig(args []string) candidateConfig {
	var cfg candidateConfig
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--master" && i+1 < len(args):
			cfg.MasterURL = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--master="):
			cfg.MasterURL = strings.TrimPrefix(args[i], "--master=")
		case args[i] == "--pair-token" && i+1 < len(args):
			cfg.PairToken = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--pair-token="):
			cfg.PairToken = strings.TrimPrefix(args[i], "--pair-token=")
		case args[i] == "--addr" && i+1 < len(args):
			cfg.Address = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--addr="):
			cfg.Address = strings.TrimPrefix(args[i], "--addr=")
		}
	}
	return cfg
}

// runCandidate starts the server in passive candidate mode:
// it accepts preflight health checks and consistent snapshots from the master,
// but does not run background loops, bots, or publish VPN configuration until promoted.
func runCandidate(dataDir string, args []string) {
	cfg := parseCandidateConfig(args)
	if len(args) == 0 {
		b, err := os.ReadFile(candidateConfigPath(dataDir))
		if err != nil {
			log.Fatalf("candidate: load config: %v", err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			log.Fatalf("candidate: parse config: %v", err)
		}
	}
	if cfg.MasterURL == "" || cfg.PairToken == "" {
		log.Fatal("candidate mode requires --master <url> and --pair-token <token>")
	}
	endpoint, err := migration.CandidateURL(cfg.Address)
	if err != nil {
		log.Fatalf("candidate: %v", err)
	}
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(endpoint, "https://"))
	addr := net.JoinHostPort("", port)

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	sm, err := migration.NewStateManager(dataDir)
	if err != nil {
		log.Fatalf("state manager: %v", err)
	}
	if sess := sm.GetSession(); sess.Role == migration.RoleMaster && sess.Phase == migration.PhaseCompleted {
		runServer(dataDir)
		return
	}
	_ = sm.SetRole(migration.RoleCandidate)
	_ = sm.SetPhase(migration.PhasePrepare)
	cert, err := migration.CandidateTLSCertificate(cfg.PairToken)
	if err != nil {
		log.Fatalf("candidate: TLS: %v", err)
	}
	securePost := func(w http.ResponseWriter, r *http.Request) bool {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Migration-Secret")), []byte(cfg.PairToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
		return true
	}

	mux := http.NewServeMux()

	// Health check for preflight validation
	mux.HandleFunc("/migration/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fmt.Appendf(nil, `{"status":"candidate_ready","version":"%s"}`, version.Version))
	})

	// Snapshot delivery endpoint
	mux.HandleFunc("/migration/apply-snapshot", func(w http.ResponseWriter, r *http.Request) {
		if !securePost(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("snapshot")
		if err != nil {
			http.Error(w, "snapshot file required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		tmp, err := os.CreateTemp(dataDir, "candidate-snap-*.tar.gz")
		if err != nil {
			http.Error(w, "temp file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)

		if _, err := tmp.ReadFrom(file); err != nil {
			_ = tmp.Close()
			http.Error(w, "save file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = tmp.Close()

		manifest, err := migration.ValidateAndExtractSnapshot(tmpPath, dataDir, "")
		if err != nil {
			http.Error(w, "validation failed: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		_ = sm.SetPhase(migration.PhaseCandidateReady)
		log.Printf("candidate: snapshot applied successfully (users: %d, domain: %s)", manifest.UsersCount, manifest.PublicDomain)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message":"snapshot applied"}`))
	})

	// Promotion endpoint
	promoted := make(chan struct{})
	var promoteOnce sync.Once
	mux.HandleFunc("/migration/promote", func(w http.ResponseWriter, r *http.Request) {
		if !securePost(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = sm.SetRole(migration.RoleMaster)
		_ = sm.SetPhase(migration.PhaseCompleted)
		log.Print("candidate: PROMOTED to active master!")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message":"promoted to master"}`))

		promoteOnce.Do(func() { close(promoted) })
	})

	srv := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}},
	}

	go func() {
		log.Printf("candidate: listening in passive mode on %s (paired with %s)", addr, cfg.MasterURL)
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("candidate http: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		log.Print("candidate: shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	case <-promoted:
		log.Print("candidate: restart into full master mode in 2 seconds...")
		time.Sleep(2 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		// Run standard master server
		runServer(dataDir)
	}
}

// runDisasterRecover restores a master in one command from a local or remote backup.
func runDisasterRecover(dataDir string, args []string) {
	var fromFile, passphrase string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--from" && i+1 < len(args):
			fromFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--from="):
			fromFile = strings.TrimPrefix(args[i], "--from=")
		case args[i] == "--passphrase" && i+1 < len(args):
			passphrase = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--passphrase="):
			passphrase = strings.TrimPrefix(args[i], "--passphrase=")
		}
	}

	if fromFile == "" {
		log.Fatal("disaster-recover: --from <backup.tar.gz> required")
	}

	report, err := backup.DisasterRecoverFromBackup(fromFile, dataDir, passphrase)
	if err != nil {
		log.Fatalf("disaster recovery failed: %v", err)
	}

	fmt.Printf("\n=== AВАРИЙНОЕ ВОССТАНОВЛЕНИЕ (DISASTER RECOVERY) ===\n")
	fmt.Printf("Публичный домен:     %s\n", report.PublicDomain)
	fmt.Printf("Секретный путь:      /%s/\n", report.SecretPath)
	fmt.Printf("Пользователей:       %d\n", report.UsersCount)
	fmt.Printf("Дата снимка:         %s\n", report.BackupCreatedAt)
	fmt.Printf("Оценка потерь (RPO): %s\n", report.EstimatedDataLoss)
	fmt.Printf("Снимок развернут в staging. Перезапустите службу: systemctl restart rospanel\n\n")
}
