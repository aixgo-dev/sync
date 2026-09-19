# Release Process

Mirrors [aixgo-dev/code](https://github.com/aixgo-dev/code) (`docs/release.md` + `internal/buildinfo`): tags are the source of truth; the binary prints the same version **without** a leading `v`.

## Versioning

Tags use semver with a leading `v` (`vX.Y.Z`). The CLI reports the stripped form (`0.1.0`).

How `aixgo-sync version` gets its value:

1. **`-ldflags`** — `make build` / `make docker` stamp from `git describe --tags --always --dirty` (override with `VERSION=…`).
2. **`go install …@vX.Y.Z`** — module build info supplies the tag when ldflags were not set.
3. **Plain `go run` / unstamped `go build`** — stays `0.0.0-dev` (same as Code’s gate builds).

## Cutting a release

Until Sync has Code-style `tag-release` / `verify-release` workflows:

1. Land the release commit on `main` with `make check` green.
2. Tag from the **remote** tip of `main` (not a stale local checkout):

   ```bash
   git fetch origin
   git checkout main
   git pull --ff-only origin main
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin tag vX.Y.Z
   ```

3. Confirm:

   ```bash
   make build
   ./bin/aixgo-sync version   # → X.Y.Z
   go install github.com/aixgo-dev/sync/cmd/aixgo-sync@vX.Y.Z
   aixgo-sync version         # → X.Y.Z
   ```

Do not retag an existing version: Go module versions are immutable once fetched by `proxy.golang.org`.

## Binary / container

- Local: `make build` → `bin/aixgo-sync`
- Image: `make docker` passes `--build-arg VERSION=…` into the Dockerfile ldflags
