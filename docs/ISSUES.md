# Aixgo Sync — issue slices for Aixgo Code

File these as GitHub issues on `aixgo-dev/sync` and apply `ax:go` after Aixgo Code is installed on the repo. Gate: `make check`.

## 1. CAS object store interface + in-memory fake

**Acceptance:** `Store` interface with Get/Put/Delete and If-Match semantics; in-memory implementation; unit tests for success, miss, and 412 conflict; no network required.

## 2. S3-compatible store adapter (R2-ready)

**Acceptance:** Adapter implementing `Store` via AWS SDK S3 API (custom endpoint); integration test skipped by default unless `SYNC_S3_TEST=1`; documents required env vars (endpoint, keys, bucket).

## 3. Tenant key layout helpers

**Acceptance:** Helpers build/parse keys for org/project/session/thread/message/job; tests reject cross-tenant path tricks (`..`, empty segments).

## 4. Stub HTTP API: healthz + sessions CRUD+CAS

**Acceptance:** `aixgo-sync serve` (or equivalent) exposes `/healthz` and session GET/PUT with ETag; table tests for CAS; `make check` still green.

## 5. Messages append + list

**Acceptance:** POST append message; GET list by thread with stable ordering; tests for tenant scoping.

## 6. Jobs create + claim

**Acceptance:** Create job queued; claim is CAS transition to claimed with worker id; double-claim fails safely; tests cover race.

Do not start deep CLI Sync-client work until issues 4–6 have landed APIs.
