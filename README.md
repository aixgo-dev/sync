# Aixgo Sync

Multi-tenant coordination plane for the Aixgo ecosystem: sessions, messages, jobs, desired-state, and remote control — durable JSON on S3-compatible storage with compare-and-swap.

**PRD:** [docs/PRD.md](docs/PRD.md) · **Issue slices:** [docs/ISSUES.md](docs/ISSUES.md) · **Ecosystem:** [aixgo-dev/product](https://github.com/aixgo-dev/product)

## Status

Early scaffold (M0). Not production-ready.

## Develop

```bash
make check
go run ./cmd/aixgo-sync version
```

## Production Docker Image (Multi-stage Scratch)

For maximum security and minimal attack surface, the production container is built using a multi-stage `Dockerfile` and runs on a minimal `scratch` image.

### Why Scratch + CA Certificates?
- **`scratch`** is an empty image, providing a minimal runtime surface with zero unneeded libraries or utilities (reducing potential CVEs).
- Since `scratch` lacks a certificate trust store, we explicitly pull fresh CA certificates from `alpine:latest` and copy them into `/etc/ssl/certs/ca-certificates.crt`.
- This ensures that outbound HTTPS/TLS requests (e.g., to S3, Cloudflare R2, or GitHub APIs) succeed seamlessly. The `SSL_CERT_FILE` environment variable is set to point to this certificate bundle.

### Build and Run Locally

To build the image using the Makefile target:
```bash
make docker
```

Or build manually:
```bash
DOCKER_BUILDKIT=1 docker build -t aixgo-sync:local .
```

To run the container locally:
```bash
docker run --rm -p 8080:8080 aixgo-sync:local
```

The server health endpoint will be available at `http://localhost:8080/healthz`.

## S3-Compatible / Cloudflare R2 Storage Config

`S3Store` implements a CAS-enabled key-value store on S3-compatible backend APIs, such as AWS S3 or Cloudflare R2. It is configured entirely via environment variables.

### Environment Variables

| Variable | Description | Example / R2 Setting |
|---|---|---|
| `SYNC_S3_ENDPOINT` | Custom API endpoint URL | `https://<account_id>.r2.cloudflarestorage.com` |
| `SYNC_S3_REGION` | S3 region | `auto` (for Cloudflare R2) or `us-east-1` |
| `SYNC_S3_BUCKET` | Target bucket name | `my-sync-bucket` |
| `SYNC_S3_ACCESS_KEY_ID` | API access key ID | `your-r2-access-key-id` |
| `SYNC_S3_SECRET_ACCESS_KEY` | API secret access key | `your-r2-secret-access-key` |
| `SYNC_S3_PREFIX` | Optional object key prefix | `sync-data/` |
| `SYNC_S3_USE_PATH_STYLE` | Optional path-style addressing | `true` (default) or `false` |

### Cloudflare R2 Working Configuration

To use Cloudflare R2 as the storage backend, configure the following environment variables:

```bash
export SYNC_S3_ENDPOINT="https://<account_id>.r2.cloudflarestorage.com"
export SYNC_S3_REGION="auto"
export SYNC_S3_BUCKET="my-sync-bucket"
export SYNC_S3_ACCESS_KEY_ID="<your-r2-access-key-id>"
export SYNC_S3_SECRET_ACCESS_KEY="<your-r2-secret-access-key>"
# Optional prefix to isolate data:
export SYNC_S3_PREFIX="sync-prod/"
```

### Running Integration Tests

To run the S3/R2 integration tests, set `SYNC_S3_TEST=1` along with the S3 configuration variables:

```bash
export SYNC_S3_TEST=1
export SYNC_S3_ENDPOINT="https://<account-id>.r2.cloudflarestorage.com"
export SYNC_S3_REGION="auto"
export SYNC_S3_BUCKET="my-test-bucket"
export SYNC_S3_ACCESS_KEY_ID="<your-access-key>"
export SYNC_S3_SECRET_ACCESS_KEY="<your-secret-key>"

make check
```

## Security & Tenant API-Key Authentication

Aixgo Sync enforces tenant isolation at the API layer using project-scoped bearer API keys.

### Authentication Header

All API requests under the `/v1/...` routes must present a valid API key in the `Authorization` header:

```http
Authorization: Bearer <token>
```

The `/healthz` endpoint remains completely open and does not require authentication.

### Local/Dev Bootstrapping

To define authorized API keys for local development or testing, configure the `SYNC_API_KEYS` environment variable. The value is a comma-separated list of `org_id:project_id:token` mappings:

```bash
export SYNC_API_KEYS="orgA:projA:secrettoken1,orgB:projB:secrettoken2"
```

### Key Security & Scope Enforcement

- **Hashed at Rest:** Plaintext tokens from `SYNC_API_KEYS` are immediately hashed using SHA-256 upon bootstrap. Only the secure hashes are stored in process memory.
- **Cross-Tenant Protection (403):** The middleware automatically parses the organization and project segments from incoming `/v1/orgs/{org}/projects/{project}/...` request paths and compares them against the authorized scope of the provided API key. If they do not match, the middleware rejects the request with a `403 Forbidden` response.
- **Context Injection:** Upon successful authentication, the authenticated `org_id` and `project_id` are propagated to downstream handlers via the request context (`auth.ContextKeyOrgID` and `auth.ContextKeyProjectID`).

## License

Apache-2.0
