# Aixgo Sync — GitHub issues for Aixgo Code

Filed from the PRD on 2026-09-19; remaining M1–M3 slices filed 2026-09-19 after v0.1.0. Gate for every issue: **`make check`** (`go test ./...`, `go vet ./...`, build `./cmd/aixgo-sync`).

Label **`ax:go`** only after Aixgo Code is installed on this repo, and **one issue at a time**.

## Done (M0 / M1 core)

| # | Issue | Milestone | Depends on |
|---|--------|-----------|------------|
| 1 | [CAS Store interface + in-memory fake](https://github.com/aixgo-dev/sync/issues/1) | M0 | — |
| 2 | [Tenant key layout helpers](https://github.com/aixgo-dev/sync/issues/2) | M0 | — |
| 3 | [S3-compatible Store adapter (R2-ready)](https://github.com/aixgo-dev/sync/issues/3) | M0/M1 | #1 |
| 4 | [HTTP serve — healthz + sessions GET/PUT](https://github.com/aixgo-dev/sync/issues/4) | M1 | #1, #2 |
| 5 | [Messages append + list](https://github.com/aixgo-dev/sync/issues/5) | M1 | #4 |
| 6 | [Jobs create + CAS claim](https://github.com/aixgo-dev/sync/issues/6) | M1 | #1, #4 |
| 7 | [Tenant API-key auth middleware (stub)](https://github.com/aixgo-dev/sync/issues/7) | M1 | #4 |
| 17 | [Packaging: multi-stage Dockerfile (scratch + CA certs)](https://github.com/aixgo-dev/sync/issues/17) | M1 | #4 |

## Remaining

| # | Issue | Milestone | Depends on |
|---|--------|-----------|------------|
| 27 | [Wire jobs API + auth middleware into serve path](https://github.com/aixgo-dev/sync/issues/27) | M1 | #4–#7 |
| 28 | [Desired-state GET/PUT (CAS)](https://github.com/aixgo-dev/sync/issues/28) | M1 | #27 |
| 29 | [Device register + list (pairings)](https://github.com/aixgo-dev/sync/issues/29) | M2 | #27 |
| 30 | [Session attach / remote-control handshake](https://github.com/aixgo-dev/sync/issues/30) | M2 | #27, #29 |
| 31 | [Notify / update plane stub (long-poll)](https://github.com/aixgo-dev/sync/issues/31) | M2 | #27 |
| 32 | [Code offload job contract + result recording](https://github.com/aixgo-dev/sync/issues/32) | M3 | #27 |
| 33 | [Cloudflare Containers deploy docs](https://github.com/aixgo-dev/sync/issues/33) | M3 | — |

## Suggested Code order

1. **#27** first (finish M1 on the running binary)
2. **#28** (desired-state)
3. **#29** then **#30** (devices → attach); **#31** can parallel attach
4. **#32** (code_offload contract) when Code integration is next
5. **#33** anytime (docs)

Defer Bot and CLI Sync-client work to their repos. Ecosystem phases M2–M4 that are CLI/Bot-owned live in [aixgo-dev/cli](https://github.com/aixgo-dev/cli) / Bot (deferred).

**Not ticketed (open questions / later):** single bucket vs bucket-per-tenant; message retention/compaction; DO SQLite for hot leases; OIDC; quotas; optional M2 audit event objects; self-host NATS runbook (ecosystem M4).

PRD: [PRD.md](./PRD.md) · Ecosystem: [PRD-ecosystem.md](https://github.com/aixgo-dev/product/blob/main/docs/PRD-ecosystem.md)
