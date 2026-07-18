# infrastructure: Responsibilities

## What It Owns

The infrastructure module provides **shared utilities** with no domain-specific business logic. Everything in the system depends on at least some of these packages.

## Packages

### `internal/loggy`

**Owns:** All logging in the system. Exclusive logging facade — no other package may call `slog.*` or `fmt.Print*` directly.

- Wraps `slog.Logger` with JSON output to stdout
- 8 semantic log categories (`"type"` field): CRITICAL, BUSINESS_LOGIC, BUSINESS_RULE_VIOLATION, API_REQUEST, API_EXTERNAL_FAILURE, API_CLIENT_ERROR, INFO, DEFAULT
- API latency tracking: `RecordApiStart()` → `ExternalApiSuccess()` / `ExternalApiFailure()` / `ApiClientError()`
- Context-enriched loggers: `loggy.With(Kv("traceId", id))` returns a new logger with additional fields
- Package-level logger instances: `var log = loggy.GetLogger("ComponentName")`

**Entry points:**
- `loggy.GetLogger(className)` — create a new logger
- `.Info()`, `.Warn()`, `.Error()` — standard levels
- `.Infoc()`, `.Warnc()`, `.Errorc()` — category-variant methods (with `context.Context`)
- `.Critical()` — semantic method for system-threatening errors
- `.RecordApiStart()`, `.ExternalApiSuccess()`, `.ExternalApiFailure()`, `.ApiClientError()` — API tracking
- `loggy.Kv(key, value)` — structured field helper

### `internal/natsutil`

**Owns:** NATS connection setup, resource provisioning, and naming constants.

- `Connect(url)` — establish NATS connection with reconnection config (10 retries, 2s wait)
- `Setup(js, spamTTL)` — provision streams + KV buckets (idempotent; used by all `cmd/*/main.go`)
- `ProvisionStreams(js)` — ensure 4 streams exist (add if missing, update if exists)
- `ProvisionKVBuckets(js, spamTTL)` — ensure 4 KV buckets exist
- `ProvisionObjectStore(js)` — ensure attachment object store exists (72h TTL)
- `ProvisionWorkerConsumer(js)` — ensure durable pull consumer exists

**Constants defined:**
- Stream names: `DISPATCH_MAILS`, `DISPATCH_AUDIT`, `DISPATCH_DEAD_LETTERS`, `DISPATCH_BOUNCES`
- Subject names: `cody.mailing.job.request.mails`, `cody.mailing.audit`, `cody.mailing.deadletter`, `cody.mailing.bounce`
- Bucket names: `senders`, `quota`, `spam`, `delivered`, `attachments`
- Consumer name: `mail-worker`

### `internal/httpsrv`

**Owns:** Shared HTTP server lifecycle for services that expose HTTP.

- `Run(ctx, name, addr, handler)` — listen until context cancel, then graceful shutdown (10s timeout)
- Used by `cmd/mail-gateway` and `cmd/mail-admin` (avoids duplicated ListenAndServe boilerplate)

### `internal/testkit`

**Owns:** Shared test doubles for unit tests (not linked into production binaries).

- `MockKV` — in-memory NATS KV (Get/Put/Create/Update/Delete/Keys) with error injection and revision tracking for CAS tests
- `WrongSeqError` — implements `nats.JetStreamError` for quota CAS conflict simulation
- Used by tests in admin, quota, sender, spam, worker, etc.

### `internal/hash`

**Owns:** SHA-256 fingerprint computation for spam deduplication.

- `SpamHash(appTag, subject, recipients[], bodyLen, htmlLen)` → hex string
- Input format: `appTag|subject|recip1,recip2|bodyLen|htmlLen`

### `internal/pii`

**Owns:** Email address masking for log safety.

- `MaskEmail(email)` → `"user@domain.com"` becomes `"u***@domain.com"`
- Single-char local part: `"a@b.com"` → `"a***@b.com"`
- Invalid email (no @): `"***"`

## Invariants

| Invariant | Enforcement |
|---|---|
| All logging goes through loggy | CLAUDE.md rule; no `slog`/`fmt.Println` imports in production code |
| PII is always masked in logs | Every log call with email uses `pii.MaskEmail()` |
| Stream provisioning is idempotent | `upsertStream()`/`upsertKV()` check existence before add/update |
| All NATS resource names are constants | No hardcoded strings outside `natsutil/setup.go` |
| Work queue stream uses WorkQueuePolicy | `DISPATCH_MAILS` configured with `Retention: WorkQueuePolicy` |

## What It Does NOT Own

- Business logic — that's in gateway, worker, services, admin, bounce
- Domain types — that's in `internal/domain`
- External API clients — that's in `internal/msgraph`
