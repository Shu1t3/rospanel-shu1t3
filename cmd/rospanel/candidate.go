package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/backup"
	"github.com/Shu1t3/rospanel-shu1t3/internal/migration"
	"github.com/Shu1t3/rospanel-shu1t3/internal/version"
)

// runCandidate starts the server in passive candidate mode:
// it accepts preflight health checks and consistent snapshots from the master,
// but does not run background loops, bots, or publish VPN configuration until promoted.
func runCandidate(dataDir string, args []string) {
	var masterURL, pairToken, addr string
	addr = "0.0.0.0:8080"

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--master" && i+1 < len(args):
			masterURL = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--master="):
			masterURL = strings.TrimPrefix(args[i], "--master=")
		case args[i] == "--pair-token" && i+1 < len(args):
			pairToken = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--pair-token="):
			pairToken = strings.TrimPrefix(args[i], "--pair-token=")
		case args[i] == "--addr" && i+1 < len(args):
			addr = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--addr="):
			addr = strings.TrimPrefix(args[i], "--addr=")
		}
	}

	if masterURL == "" || pairToken == "" {
		log.Fatal("candidate mode requires --master <url> and --pair-token <token>")
	}

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	sm, err := migration.NewStateManager(dataDir)
	if err != nil {
		log.Fatalf("state manager: %v", err)
	}
	_ = sm.SetRole(migration.RoleCandidate)
	_ = sm.SetPhase(migration.PhasePrepare)

	mux := http.NewServeMux()

	// Health check for preflight validation
	mux.HandleFunc("/migration/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fmt.Appendf(nil, `{"status":"candidate_ready","version":"%s"}`, version.Version))
	})

	// Snapshot delivery endpoint
	mux.HandleFunc("/migration/apply-snapshot", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/migration/promote", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = sm.SetRole(migration.RoleMaster)
		_ = sm.SetPhase(migration.PhaseCompleted)
		log.Print("candidate: PROMOTED to active master!")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message":"promoted to master"}`))

		close(promoted)
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("candidate: listening in passive mode on %s (paired with %s)", addr, masterURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
