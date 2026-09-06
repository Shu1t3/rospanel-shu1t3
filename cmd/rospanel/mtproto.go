package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
	"github.com/Shu1t3/rospanel-shu1t3/internal/version"
)

func runMTProto(args []string) {
	if len(args) == 0 {
		printMTProtoUsage(os.Stderr)
		os.Exit(2)
	}

	switch args[0] {
	case "run":
		runMTProtoProxy(args[1:])
	case "gen-secret":
		runMTProtoGenSecret(args[1:])
	case "help", "--help", "-h":
		printMTProtoUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown mtproto command %q\n\n", args[0])
		printMTProtoUsage(os.Stderr)
		os.Exit(2)
	}
}

func printMTProtoUsage(w io.Writer) {
	fmt.Fprint(w, `rospanel mtproto `+version.Version+` — Embedded MTProto FakeTLS Proxy

Usage:
  rospanel mtproto run [flags]
  rospanel mtproto gen-secret [--domain <domain>]

Subcommands:
  run          Run the standalone MTProto proxy (ultra-low memory mode, < 30 MB RSS).
               Only starts the proxy and optional control-plane sync; does NOT start
               Xray, SQLite, or web panel services.
  gen-secret   Generate a random FakeTLS secret string for a fronting domain.

Flags for 'run':
  --port <port>         TCP port to bind (default: 8443 or ROSPANEL_MTPROTO_PORT)
  --secret <secret>     FakeTLS secret (hex or base64, starts with 'ee')
  --domain <domain>     SNI fronting domain (default: cloudflare.com)
  --max-conns <conns>   Max concurrent connections (default: 512)
  --sync-url <url>      RosPanel control-plane sync URL (e.g. https://panel.example.com/api/mtproto/sync)
  --sync-token <token>  Bearer token for control-plane authentication
  --pprof-addr <addr>   Optional HTTP address for runtime pprof profiling (e.g. :6060)

Environment variables:
  ROSPANEL_MTPROTO_PORT, ROSPANEL_MTPROTO_SECRET, ROSPANEL_MTPROTO_DOMAIN,
  ROSPANEL_MTPROTO_MAX_CONNS, ROSPANEL_MTPROTO_SYNC_URL, ROSPANEL_MTPROTO_SYNC_TOKEN,
  ROSPANEL_PPROF_ADDR
`)
}

func runMTProtoGenSecret(args []string) {
	fs := flag.NewFlagSet("gen-secret", flag.ExitOnError)
	domain := fs.String("domain", "cloudflare.com", "FakeTLS SNI fronting domain")
	_ = fs.Parse(args)

	sec, err := mtproto.GenerateSecret(*domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate secret: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Domain: %s\n", *domain)
	fmt.Printf("Secret: %s\n", sec)
}

func runMTProtoProxy(args []string) {
	envCfg := mtproto.ParseConfigFromEnv()

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	port := fs.Int("port", envCfg.Port, "TCP port to bind")
	secret := fs.String("secret", envCfg.Secret, "FakeTLS secret")
	domain := fs.String("domain", envCfg.Domain, "SNI fronting domain")
	maxConns := fs.Uint("max-conns", envCfg.MaxConns, "Maximum concurrent connections")
	syncURL := fs.String("sync-url", envCfg.RemoteSyncURL, "Control-plane sync endpoint")
	syncToken := fs.String("sync-token", envCfg.SyncToken, "Control-plane sync bearer token")
	pprofAddr := fs.String("pprof-addr", envCfg.PprofAddr, "Runtime pprof HTTP address (e.g. :6060)")
	_ = fs.Parse(args)

	cfg := envCfg
	if *port > 0 {
		cfg.Port = *port
	}
	if *secret != "" {
		cfg.Secret = strings.TrimSpace(*secret)
	}
	if *domain != "" {
		cfg.Domain = strings.TrimSpace(*domain)
	}
	if *maxConns > 0 {
		cfg.MaxConns = *maxConns
	}
	if *syncURL != "" {
		cfg.RemoteSyncURL = strings.TrimSpace(*syncURL)
	}
	if *syncToken != "" {
		cfg.SyncToken = strings.TrimSpace(*syncToken)
	}
	if *pprofAddr != "" {
		cfg.PprofAddr = strings.TrimSpace(*pprofAddr)
	}

	// If secret is missing and standalone without sync, generate an ephemeral one
	if cfg.Secret == "" && cfg.RemoteSyncURL == "" {
		sec, err := mtproto.GenerateSecret(cfg.Domain)
		if err != nil {
			slog.Error("failed to generate FakeTLS secret", "err", err)
			os.Exit(1)
		}
		cfg.Secret = sec
		slog.Info("no secret provided, generated ephemeral secret", "secret", sec, "domain", cfg.Domain)
	}

	sup, err := mtproto.NewLifecycle(cfg)
	if err != nil {
		slog.Error("initialize MTProto lifecycle error", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGHUP:
				slog.Info("received SIGHUP, reloading configuration")
				// SIGHUP triggers a reload from environment or sync
				reloadedCfg := mtproto.ParseConfigFromEnv()
				if err := sup.Reload(reloadedCfg); err != nil {
					slog.Warn("reload failed", "err", err)
				}
			case os.Interrupt, syscall.SIGTERM:
				slog.Info("shutting down MTProto proxy...")
				cancel()
				_ = sup.Close()
				return
			}
		}
	}()

	// Start control-plane sync if configured
	if cfg.RemoteSyncURL != "" && cfg.SyncToken != "" {
		cp := mtproto.NewControlPlane(sup)
		cp.Start(ctx)
		defer cp.Stop()
	}

	slog.Info("starting RosPanel standalone MTProto proxy",
		"port", cfg.Port,
		"domain", cfg.Domain,
		"max_conns", cfg.MaxConns,
		"sync_url", cfg.RemoteSyncURL,
		"pprof", cfg.PprofAddr,
	)

	if err := sup.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("MTProto proxy exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("MTProto proxy stopped cleanly")
}
