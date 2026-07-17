# infrastructure: Responsibilities

## What It Owns

The infrastructure module provides **shared utilities** with no domain-specific business logic. Everything in the system depends on at least some of these packages.

## Packages

### `internal/loggy`

**Owns:** All logging in the system. Exclusive logging facade — no other package may call `slog.*` or `fmt.Print*` directly.

- Wraps `slog.Logger` with JSON output to stdout
- 14 semantic log categories (`"type"` field): CRITICAL, BUSINESS_LOGIC, VALIDATION, API_REQUEST, SECURITY, etc.
- API latency tracking: `RecordApiStart()` → `ExternalApiSuccess()` / `ExternalApiFailure()`
- Context-enriched loggers: `loggy.With(Kv("traceId", id))` returns a new logger with additional fields
- Package-level logger instances: `var log = loggy.GetLogger("ComponentName")`

**Entry points:**
- `loggy.GetLogger(className)` — create a new logger
- `.Info()`, `.Warn()`, `.Error()`, `.Debug()` — standard levels
- `.Infoc()`, `.Warnc()`, `.Errorc()`, `.Debugc()` — category-variant methods
- `.Critical()`, `.BusinessRuleViolation()`, `.ValidationFailed()`, `.MissingData()` — semantic methods
- `.RecordApiStart()`, `.ExternalApiSuccess()`, `.ExternalApiFailure()`, `.ApiClientError()` — API tracking

### `internal/natsutil`

**Owns:** NATS connection setup, resource provisioning, and naming constants.

- `Connect(url)` — establish NATS connection with reconnection config (10 retries, 2s wait)
- `ProvisionStreams(js)` — ensure 4 streams exist (idempotent: add if missing, update if exists)
- `ProvisionKVBuckets(js, spamTTL)` — ensure 4 KV buckets exist
- `ProvisionObjectStore(js)` — ensure attachment object store exists (72h TTL)
- `ProvisionWorkerConsumer(js)` — ensure durable pull consumer exists

**Constants defined:**
- Stream names: `DISPATCH_MAILS`, `DISPATCH_AUDIT`, `DISPATCH_DEAD_LETTERS`, `DISPATCH_BOUNCES`
- Subject names: `cody.mailing.job.request.mails`, `cody.mailing.audit`, `cody.mailing.deadletter`, `cody.mailing.bounce`
- Bucket names: `senders`, `quota`, `spam`, `delivered`, `attachments`
- Consumer name: `mail-worker`

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
