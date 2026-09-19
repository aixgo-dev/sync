# Aixgo Sync PRD

**Status:** Draft v0.1 · 2026-09-19  
**Repo:** [aixgo-dev/sync](https://github.com/aixgo-dev/sync)  
**Umbrella:** [product/docs/PRD-ecosystem.md](https://github.com/aixgo-dev/product/blob/main/docs/PRD-ecosystem.md)

## 1. Problem

Aixgo needs a shared coordination plane so humans, local CLI workers, Aixgo Code jobs, and (later) Bot can:

- share session state and messages across devices
- enqueue and claim jobs (including offload to Code / GitHub)
- remotely control a running agent session
- do this multi-tenant (orgs, projects, future companies)

without a heavy database or CRDT document model.

## 2. Goals

- Multi-actor coordination (humans + independent agents)
- Durable JSON state on S3-compatible storage with **CAS**
- Thin HTTPS CRUD API runnable in Cloudflare Containers
- Portable contract (self-host later on any S3 + optional NATS)
- First-class tenancy: org → project → resources
- Native remote control via session attach

## 3. Non-goals (v1)

- CRDT collaborative editing core
- Full SQL query plane / analytics warehouse
- Shipping MinIO server in-tree
- Replacing Aixgo Code’s forge loop (Sync *triggers* Code via jobs)
- Bot product UI

## 4. Users / actors

| Actor | Needs |
|-------|--------|
| Human (CLI/web/Bot later) | Messages, remote attach, enqueue work |
| CLI worker | Register session, receive commands, post tool/results, create offload jobs |
| Aixgo Code / GitHub worker | Claim offload jobs, report PR URLs / status |
| Admin / org owner | Tenants, API keys, quotas |

## 5. Tenancy

```
tenant (org)
  └── project
        ├── sessions
        ├── threads / mailboxes
        ├── jobs
        ├── desired-state docs
        └── devices / pairings
```

Every object key and auth token is scoped to org + project. No flat global namespace.

## 6. Object model (JSON sketches)

### 6.1 Key layout (illustrative)

```
tenants/{org_id}/projects/{project_id}/meta.json
tenants/{org_id}/projects/{project_id}/sessions/{session_id}.json
tenants/{org_id}/projects/{project_id}/threads/{thread_id}/meta.json
tenants/{org_id}/projects/{project_id}/threads/{thread_id}/messages/{msg_id}.json
tenants/{org_id}/projects/{project_id}/jobs/{job_id}.json
tenants/{org_id}/projects/{project_id}/desired/{name}.json
tenants/{org_id}/projects/{project_id}/devices/{device_id}.json
```

### 6.2 Common envelope

```json
{
  "id": "…",
  "org_id": "…",
  "project_id": "…",
  "type": "session|message|job|…",
  "created_at": "RFC3339",
  "updated_at": "RFC3339",
  "body": {}
}
```

CAS: clients send `If-Match: <etag>` on updates; store returns new etag on success; `412` on conflict → client re-reads and retries.

### 6.3 Session

Identity for a live or hibernated agent conversation. Fields: `status` (open|closed), `worker_id` (optional), `thread_id`, `labels`.

### 6.4 Message

Append-mostly. Fields: `thread_id`, `sender_actor_id`, `role` (user|assistant|system|tool|agent), `content`, `refs`.

### 6.5 Job

Fields: `kind` (code_offload|command|schedule|custom), `status` (queued|claimed|running|done|failed|cancelled), `payload`, `assignee_worker`, `result`.

### 6.6 Desired-state

Named document workers reconcile against (JumpCloud/Orbit pattern).

## 7. API surface (sketch)

All under `/v1/orgs/{org}/projects/{project}/…` with bearer/API key auth.

- `GET/PUT` sessions, jobs, desired-state (CAS on PUT)
- `POST` messages (append); `GET` list/page messages
- `POST` jobs; `POST` jobs/{id}/claim
- `POST` sessions/{id}/attach — remote control handshake
- `GET /healthz`

Update plane (notify “work available”): interface stub in M0; CF Queues or long-poll in M1; NATS optional for self-host.

## 8. Remote control flow

1. CLI worker registers device + session with Sync  
2. Remote client authenticates to same org/project and attaches to session  
3. Commands land as jobs or control messages on the session mailbox  
4. Worker reconciles outbound (no inbound ports required on the laptop)

## 9. Offload to Aixgo Code

1. CLI/human creates job `kind=code_offload` with repo, issue body, gate hints  
2. Code worker (or human filing `ax:go`) claims job  
3. Job result stores PR URL / status  
4. Session mailbox notified

## 10. Hosting

| Layer | v1 | Self-host later |
|-------|----|-----------------|
| Compute | Cloudflare Containers (Go binary) | Any Linux/container |
| State | R2 via S3 API + CAS | S3/GCS/R2-compatible |
| Notify | CF Queues / long-poll | NATS JetStream or long-poll |

## 11. Security

- Per-org API keys (hashed at rest); OIDC later  
- Least privilege: project-scoped tokens  
- No secrets in object bodies; reference external secret stores  
- Audit: append-only event objects optional in M2

## 12. Milestones

| ID | Deliverable | Acceptance |
|----|-------------|------------|
| **M0** | Module + stub binary + store interface + PRD | `make check` green; interface tested with memory/fake S3 |
| **M1** | HTTPS API sessions/messages/jobs + tenant auth stub + R2 | CAS conflict test; multi-tenant key isolation test |
| **M2** | Attach/remote-control + job claim + notify | Two clients share one session mailbox |
| **M3** | Code offload path + Containers deploy docs | End-to-end job → PR URL recorded |

## 13. Open questions

- Single bucket with prefixes vs bucket-per-tenant  
- Message retention / compaction policy  
- Whether DO SQLite is ever used for hot leases (lean no for source of truth)

## 13b. License

Apache-2.0.

## 14. References

- Ecosystem PRD in `aixgo-dev/product`  
- Issue slices: [ISSUES.md](./ISSUES.md)
