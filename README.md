# Aixgo Sync

Multi-tenant coordination plane for the Aixgo ecosystem. Durable JSON for sessions, messages, and jobs — on memory or S3-compatible storage with compare-and-swap — built so agents, CLI workers, and Code can share state, enqueue work, and (next) drive desired-state and remote control.

**PRD:** [docs/PRD.md](docs/PRD.md) · **Issue slices:** [docs/ISSUES.md](docs/ISSUES.md) · **Ecosystem:** [aixgo-dev/product](https://github.com/aixgo-dev/product)

## Status

**v0.1.0** — first release of the M0/M1 coordination core (CAS store, tenant keys, HTTP sessions/messages, jobs + tenant API-key auth packages). Session attach, notify plane, and Code offload end-to-end come later.

## What’s in this release

- **CAS store** — durable JSON objects in memory or on S3-compatible backends (Cloudflare R2 ready), with compare-and-swap / ETag
- **Tenant key layout** — org → project object keys for sessions, thread messages, jobs, and desired-state docs
- **HTTP API (serve path)**
  - `GET /healthz`
  - Sessions: `GET` / `PUT` with CAS via `If-Match` / ETag
  - Thread messages: `POST` append, `GET` list
- **Jobs + auth packages** — jobs create/get/CAS claim and project-scoped bearer API keys (`SYNC_API_KEYS`, SHA-256 hashed in memory, cross-tenant `403`) are implemented and tested; wire them into the serve mux as you harden the binary
- **Packaging** — multi-stage `scratch` image with CA certs for outbound TLS; `make check`, `make build`, `make docker`

## Develop

```bash
make check
make build
./bin/aixgo-sync version   # stamped from git describe (e.g. 0.1.0)
```

`make build` / `make docker` inject the version via `-ldflags` (same pattern as [aixgo-dev/code](https://github.com/aixgo-dev/code)). Plain `go run` stays `0.0.0-dev`. See [docs/release.md](docs/release.md).

Default serve uses an in-memory store. Set `SYNC_S3_BUCKET` (and related vars below) to back the process with S3/R2. Listen port: `SYNC_PORT` or `PORT` (default `8080`).

```bash
go run ./cmd/aixgo-sync serve
# or
go run ./cmd/aixgo-sync
```

## Docker (scratch)

The production image is a multi-stage build that copies a static binary into `scratch` plus a CA bundle for outbound HTTPS (S3/R2 and similar).

Why scratch + CA certs:

- `scratch` keeps the runtime surface minimal — no shell, package manager, or unused libraries
- Alpine supplies `/etc/ssl/certs/ca-certificates.crt`; `SSL_CERT_FILE` points at that bundle so TLS to R2/S3 works

```bash
make docker
# or
docker build -t aixgo-sync:local .

docker run --rm -p 8080:8080 aixgo-sync:local
```

Health: `http://localhost:8080/healthz`.

## S3-compatible / Cloudflare R2

`S3Store` is a CAS key-value store on S3-compatible APIs. Configure it with environment variables. If `SYNC_S3_BUCKET` is unset, the server uses the in-memory store.

### Environment variables

| Variable | Description | Example / R2 setting |
|---|---|---|
| `SYNC_S3_ENDPOINT` | Custom API endpoint URL | `https://<account_id>.r2.cloudflarestorage.com` |
| `SYNC_S3_REGION` | S3 region | `auto` (R2) or `us-east-1` |
| `SYNC_S3_BUCKET` | Target bucket name | `my-sync-bucket` |
| `SYNC_S3_ACCESS_KEY_ID` | API access key ID | `your-r2-access-key-id` |
| `SYNC_S3_SECRET_ACCESS_KEY` | API secret access key | `your-r2-secret-access-key` |
| `SYNC_S3_PREFIX` | Optional object key prefix | `sync-data/` |
| `SYNC_S3_USE_PATH_STYLE` | Optional path-style addressing | `true` (default) or `false` |

### Cloudflare R2 example

```bash
export SYNC_S3_ENDPOINT="https://<account_id>.r2.cloudflarestorage.com"
export SYNC_S3_REGION="auto"
export SYNC_S3_BUCKET="my-sync-bucket"
export SYNC_S3_ACCESS_KEY_ID="<your-r2-access-key-id>"
export SYNC_S3_SECRET_ACCESS_KEY="<your-r2-secret-access-key>"
# Optional prefix:
export SYNC_S3_PREFIX="sync-prod/"
```

### Integration tests

Set `SYNC_S3_TEST=1` plus the S3/R2 vars, then run the suite:

```bash
export SYNC_S3_TEST=1
export SYNC_S3_ENDPOINT="https://<account-id>.r2.cloudflarestorage.com"
export SYNC_S3_REGION="auto"
export SYNC_S3_BUCKET="my-test-bucket"
export SYNC_S3_ACCESS_KEY_ID="<your-access-key>"
export SYNC_S3_SECRET_ACCESS_KEY="<your-secret-key>"

make check
```

## Auth (tenant API keys)

Project-scoped bearer keys live in `internal/auth`. When the middleware is wrapped around the mux, `/v1/...` routes expect:

```http
Authorization: Bearer <token>
```

`/healthz` stays open (no auth).

### Local bootstrap

`SYNC_API_KEYS` is a comma-separated list of `org_id:project_id:token` entries:

```bash
export SYNC_API_KEYS="orgA:projA:secrettoken1,orgB:projB:secrettoken2"
```

### Scope enforcement

- **Hashed at rest** — plaintext tokens from `SYNC_API_KEYS` are SHA-256 hashed at bootstrap; only hashes live in process memory
- **Cross-tenant 403** — middleware compares the path’s `/v1/orgs/{org}/projects/{project}/...` segments to the key’s org+project scope; mismatch → `403 Forbidden`
- **Context** — on success, `org_id` and `project_id` are available to handlers via `auth.ContextKeyOrgID` and `auth.ContextKeyProjectID`

## Docs

- Product requirements: [docs/PRD.md](docs/PRD.md)
- Implementation slices: [docs/ISSUES.md](docs/ISSUES.md)
- Ecosystem overview: [aixgo-dev/product](https://github.com/aixgo-dev/product)

## License

Apache-2.0
