# mail-gateway: Responsibilities

## What It Owns

The mail gateway is the **system ingress** — it accepts mail send requests from external clients, validates them through a 7-stage pipeline, and publishes them to NATS JetStream for processing by the mail-worker.

## Entry Points

| Entry Point | Type | Purpose |
|---|---|---|
| `POST /dispatch/api/v1/mail/send` | HTTP endpoint | Accept mail send requests |
| `GET /health` | HTTP endpoint | Health check (`{"status":"UP","checks":[...]}`) |
| `GET /health/live` | HTTP endpoint | Liveness probe (always 200) |
| `GET /health/ready` | HTTP endpoint | Readiness probe (same as /health) |

## 7-Stage Pipeline (in order — execution is sequential and must not be reordered)

| # | Stage | File | What Happens | Failure |
|---|---|---|---|---|
| 1 | JSON decode + struct validation | `gateway/handler.go:92-100` + `gateway/validation.go` | Decode JSON → `domain.MailRequest`, validate struct tags, body size, MIME whitelist, attachment size, base64 correctness | HTTP 400 or 413 |
| 2 | Sender lookup | `gateway/handler.go:102-106` | `sender.Store.Get(appTag)` → NATS KV with 10-min cache | HTTP 400 (unknown appTag) |
| 3 | Domain whitelist | `gateway/handler.go:108-120` | Check all recipient domains against `sender.AllowedDomains` | HTTP 400 |
| 4 | Quota check | `gateway/handler.go:122-126` | `quota.Checker.Check(appTag, limit, count)` → rolling 24h CAS | HTTP 429 or 503 |
| 5 | Spam dedup | `gateway/handler.go:128-132` | `spam.Checker.Check(hash)` → SHA-256 fingerprint in TTL bucket | HTTP 400 |
| 6 | Attachment upload | `gateway/handler.go:134-142` | Streaming base64 decode → NATS Object Store | HTTP 503 |
| 7 | NATS publish | `gateway/handler.go:155-165` | Publish `MailRequestDO` to `DISPATCH_MAILS` | HTTP 503 |

## Invariants

| Invariant | Enforcement |
|---|---|
| Send endpoint requires Bearer auth | Middleware on route group; health outside; startup fails without token unless disabled |
| Pipeline stages execute in order, never skip | `handleSend` is a single function with sequential stages |
| Invalid requests never reach NATS | Early return on any validation error |
| Quota errors are fail-closed | Any KV error → `QuotaStateError` → HTTP 503 |
| NATS publish errors are fail-closed | Any publish error → `NatsPublishError` → HTTP 503 |
| PII is masked in all log statements | All log calls use `pii.MaskEmail()` for email addresses |
| Response always includes `traceId` | `uuid.New()` at function entry; included in all error and success responses |
| Request body size is limited before JSON decode | `http.MaxBytesReader` wraps the request body |

## What It Does NOT Own

- Email delivery to MS Graph — that's the mail-worker
- Sender configuration management — that's mail-admin (gateway only reads)
- Quota state eviction — NATS KV TTL handles this
- Attachment cleanup after delivery — mail-worker does this
