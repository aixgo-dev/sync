package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

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

	srv := server.NewServer(activeStore)
	log.Printf("Starting aixgo-sync server on %s (version: %s)...", addr, version.Version)

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
