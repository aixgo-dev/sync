.PHONY: check build docker
check:
	go test ./...
	go vet ./...
	go build -o /dev/null ./cmd/aixgo-sync

build:
	go build -o bin/aixgo-sync ./cmd/aixgo-sync

docker:
	docker build -t aixgo-sync:local .
