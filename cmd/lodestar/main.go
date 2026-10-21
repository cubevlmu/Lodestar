package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"lodestar/internal/agent"
	"lodestar/internal/api"
	"lodestar/internal/buildinfo"
	"lodestar/internal/config"
	"lodestar/internal/db"
	"lodestar/internal/security"

	"gopkg.in/yaml.v3"
)

func main() {
	log.SetFlags(log.LstdFlags)

	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	flags := flag.NewFlagSet("lodestar", flag.ExitOnError)
	configPath := flags.String("config", "", "path to system.yaml")
	addr := flags.String("addr", "", "override listen address")
	_ = flags.Parse(args)

	if *configPath != "" {
		os.Setenv("LODESTAR_CONFIG", *configPath)
	}
	cfg := config.Load()
	if *addr != "" {
		cfg.Addr = *addr
	}

	switch command {
	case "serve", "server":
		runServer(cfg)
	case "agent":
		runAgent(cfg)
	case "genkeys":
		printKeys()
	case "hash":
		printHash(flags.Args())
	case "version":
		fmt.Printf("lodestar %s (%s, %s/%s)  ·  %s\n", buildinfo.Version, buildinfo.Commit, runtime.GOOS, runtime.GOARCH, buildinfo.Author)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\nusage: lodestar [serve|agent|genkeys|hash|version] [-config path]\n", command)
		os.Exit(2)
	}
}

func runServer(cfg config.Config) {
	ensureSystemConfig(cfg)

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	username, password, created, err := api.EnsureBootstrapAdmin(database, cfg)
	if err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	router := api.NewRouter(database, cfg)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Println()
	fmt.Println("  ██╗      ██████╗ ██████╗ ███████╗███████╗████████╗ █████╗ ██████╗")
	fmt.Println("  ██║     ██╔═══██╗██╔══██╗██╔════╝██╔════╝╚══██╔══╝██╔══██╗██╔══██╗")
	fmt.Println("  ██║     ██║   ██║██║  ██║█████╗  ███████╗   ██║   ███████║██████╔╝")
	fmt.Println("  ██║     ██║   ██║██║  ██║██╔══╝  ╚════██║   ██║   ██╔══██║██╔══██╗")
	fmt.Println("  ███████╗╚██████╔╝██████╔╝███████╗███████║   ██║   ██║  ██║██║  ██║")
	fmt.Println("  ╚══════╝ ╚═════╝ ╚═════╝ ╚══════╝╚══════╝   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝")
	fmt.Printf("\n  lodestar %s  ·  release control plane\n", buildinfo.Version)
	fmt.Printf("  WebUI   http://%s\n", displayAddr(cfg.Addr))
	fmt.Printf("  Data    %s  ·  Storage %s\n", cfg.DBPath, cfg.StorageDir)
	if created {
		fmt.Printf("\n  Initial administrator created\n    username: %s\n    password: %s\n  Change it after the first sign-in.\n", username, password)
	}
	fmt.Println()

	log.Printf("[server] listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
	log.Printf("[server] stopped")
}

func runAgent(cfg config.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := agent.Run(ctx, cfg.Agent); err != nil {
		log.Fatalf("agent: %v", err)
	}
}

func printKeys() {
	clientSecret, err := security.RandomBase64(32)
	if err != nil {
		log.Fatalf("generate client secret: %v", err)
	}
	pub, priv, err := security.GenerateEd25519KeyPair()
	if err != nil {
		log.Fatalf("generate ed25519 keys: %v", err)
	}
	fmt.Println("# Paste into a project record (WebUI creates these automatically).")
	fmt.Printf("client_secret: %q\n", clientSecret)
	fmt.Printf("ed25519_public_key: %q\n", pub)
	fmt.Printf("ed25519_private_key: %q\n", priv)
	fmt.Printf("deploy_token: %q\n", security.NewToken(32))
}

func printHash(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lodestar hash <file>")
		os.Exit(2)
	}
	file, err := os.Open(args[0])
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer file.Close()
	sum, size, err := security.SHA256FileHex(file)
	if err != nil {
		log.Fatalf("hash: %v", err)
	}
	fmt.Printf("sha256: %s\nsize:   %d\n", sum, size)
}

// ensureSystemConfig writes a usable config on first run so secrets are stable.
func ensureSystemConfig(cfg config.Config) {
	path := os.Getenv("LODESTAR_CONFIG")
	if path == "" {
		path = "data/config/system.yaml"
	}
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[config] cannot create %s: %v", filepath.Dir(path), err)
		return
	}
	secret := cfg.DownloadTokenSecret
	if strings.HasPrefix(secret, "change-me") || secret == "" {
		secret = security.NewToken(32)
		cfg.DownloadTokenSecret = secret
	}
	payload := map[string]any{
		"addr":                       cfg.Addr,
		"db_path":                    cfg.DBPath,
		"storage_dir":                cfg.StorageDir,
		"data_dir":                   cfg.DataDir,
		"public_base_url":            cfg.PublicBaseURL,
		"download_token_secret":      secret,
		"client_clock_skew_seconds":  int(cfg.ClientClockSkew.Seconds()),
		"download_token_ttl_seconds": int(cfg.DownloadTokenTTL.Seconds()),
		"session_ttl_hours":          int(cfg.SessionTTL.Hours()),
		"max_upload_mb":              cfg.MaxUploadBytes / (1024 * 1024),
		"admin_username":             cfg.AdminUsername,
	}
	data, err := yaml.Marshal(payload)
	if err != nil {
		log.Printf("[config] marshal defaults: %v", err)
		return
	}
	header := []byte("# Lodestar system configuration (generated on first run)\n")
	if err := os.WriteFile(path, append(header, data...), 0o600); err != nil {
		log.Printf("[config] write %s: %v", path, err)
	}
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
