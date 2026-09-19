.PHONY: check build docker

# Strip leading v from git tags so the binary matches aixgo-dev/code
# (aixgo-code version prints 0.1.0, not v0.1.0). Override with VERSION=...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo 0.0.0-dev)
VERSION_PKG := github.com/aixgo-dev/sync/internal/version
LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION)

check:
	go test ./...
	go vet ./...
	go build -o /dev/null ./cmd/aixgo-sync

build:
	go build -ldflags="$(LDFLAGS)" -o bin/aixgo-sync ./cmd/aixgo-sync

docker:
	docker build --build-arg VERSION=$(VERSION) -t aixgo-sync:local .
