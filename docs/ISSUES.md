# Aixgo Sync — GitHub issues for Aixgo Code

Filed from the PRD on 2026-09-19. Gate for every issue: **`make check`** (`go test ./...`, `go vet ./...`, build `./cmd/aixgo-sync`).

Label **`ax:go`** only after Aixgo Code is installed on this repo, and **one issue at a time** (start at #1).

| # | Issue | Milestone | Depends on |
|---|--------|-----------|------------|
| 1 | [CAS Store interface + in-memory fake](https://github.com/aixgo-dev/sync/issues/1) | M0 | — |
| 2 | [Tenant key layout helpers](https://github.com/aixgo-dev/sync/issues/2) | M0 | — |
| 3 | [S3-compatible Store adapter (R2-ready)](https://github.com/aixgo-dev/sync/issues/3) | M0/M1 | #1 |
| 4 | [HTTP serve — healthz + sessions GET/PUT](https://github.com/aixgo-dev/sync/issues/4) | M1 | #1, #2 |
| 5 | [Messages append + list](https://github.com/aixgo-dev/sync/issues/5) | M1 | #4 |
| 6 | [Jobs create + CAS claim](https://github.com/aixgo-dev/sync/issues/6) | M1 | #1, #4 |
| 7 | [Tenant API-key auth middleware (stub)](https://github.com/aixgo-dev/sync/issues/7) | M1 | #4 |

## Suggested Code order

1. #1 and #2 (parallel OK)
2. #3 (optional early; can wait until after #4 if you want HTTP first)
3. #4
4. #7 (auth before exposing serve beyond localhost)
5. #5 and #6 (parallel OK after #4)

Defer CLI Sync-client work until #4–#6 have landed APIs.

PRD: [PRD.md](./PRD.md)
