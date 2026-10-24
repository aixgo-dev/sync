package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/aixgo-dev/sync/internal/auth"
	"github.com/aixgo-dev/sync/internal/server"
	"github.com/aixgo-dev/sync/internal/store"
	"github.com/aixgo-dev/sync/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.Version)
		return
	}

	// We support "serve" subcommand or starting serve directly by default.
	isServe := true
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		isServe = false
	}

	if !isServe {
		fmt.Fprintf(os.Stderr, "aixgo-sync %s — coordination plane server\n", version.Version)
		fmt.Fprintf(os.Stderr, "usage: aixgo-sync [serve | version]\n")
		os.Exit(1)
	}

	ctx := context.Background()

	var activeStore store.Store
	var err error

	// If S3 bucket is configured, initialize S3Store; otherwise fall back to MemoryStore
	if os.Getenv("SYNC_S3_BUCKET") != "" {
		log.Println("Initializing S3Store from environment...")
		activeStore, err = store.NewS3StoreFromEnv(ctx)
		if err != nil {
			log.Fatalf("Failed to initialize S3Store: %v", err)
		}
	} else {
		log.Println("Initializing in-memory MemoryStore...")
		activeStore = store.NewMemoryStore()
	}

	// Read port from SYNC_PORT or PORT, default to :8080
	port := os.Getenv("SYNC_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	// Format host:port cleanly
	addr := net.JoinHostPort("0.0.0.0", port)

	handler, err := buildHandler(activeStore, os.Getenv)
	if err != nil {
		log.Fatalf("Refusing to start: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Starting aixgo-sync server on %s (version: %s)...", addr, version.Version)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// Environment variables controlling authentication.
const (
	// envAPIKeys holds comma-separated org:project:token entries.
	envAPIKeys = "SYNC_API_KEYS"
	// envInsecureNoAuth, when set to "1" or "true", lets the server start with
	// no API keys and no authentication. Intended for local development only.
	envInsecureNoAuth = "SYNC_INSECURE_NO_AUTH"
)

// buildHandler assembles the HTTP handler served by the binary. All /v1 routes
// are wrapped in API-key authentication; /healthz stays open. If no API keys
// are configured it returns an error, unless SYNC_INSECURE_NO_AUTH is set, in
// which case authentication is disabled and a warning is logged.
func buildHandler(st store.Store, getenv func(string) string) (http.Handler, error) {
	api := server.NewServer(st).Handler()

	keyStore, err := auth.NewKeyStore(getenv(envAPIKeys))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", envAPIKeys, err)
	}

	if keyStore.Len() == 0 {
		switch getenv(envInsecureNoAuth) {
		case "1", "true":
			log.Printf("WARNING: %s is set and no %s configured: authentication is DISABLED. Use for local development only.", envInsecureNoAuth, envAPIKeys)
			return api, nil
		default:
			return nil, fmt.Errorf("no API keys configured: set %s (org:project:token,...) or, for local development only, set %s=1", envAPIKeys, envInsecureNoAuth)
		}
	}

	return keyStore.Middleware(api), nil
}
