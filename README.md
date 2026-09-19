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

## License

Apache-2.0
