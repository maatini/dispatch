# Components

## mail-gateway

**What it is:** HTTP REST entrypoint for mail send requests.

**Responsibilities:**
- Accept `POST /dispatch/api/v1/mail/send` with JSON body (domain.MailRequest)
- Run a 7-stage validation pipeline: JSON decode → struct validation → sender lookup → domain whitelist → quota check → spam dedup → attachment upload
- Publish `MailRequestDO` to NATS JetStream `DISPATCH_MAILS`
- Serve `/health`, `/health/live`, `/health/ready`

**Key files:** `cmd/mail-gateway/main.go`, `internal/gateway/handler.go`, `internal/gateway/validation.go`, `internal/gateway/publisher.go`, `internal/gateway/attachstore.go`

**Module docs:** [modules/mail-gateway/](../modules/mail-gateway/index.md)

---

## mail-worker

**What it is:** NATS JetStream pull consumer that delivers emails via MS Graph.

**Responsibilities:**
- Consume from `DISPATCH_MAILS` stream (durable consumer `mail-worker`, explicit ack, 30s ack-wait)
- Deserialize MailRequestDO → dedup check (KV `delivered`) → fetch attachments → send via MS Graph → write audit record
- Error handling: transient errors (429/5xx) → no ACK (redelivery); permanent errors (4xx) → ACK + FAILED audit; malformed JSON → ACK + dead letter
- Cleanup attachment objects after successful or hard-failed delivery

**Key files:** `cmd/mail-worker/main.go`, `internal/worker/consumer.go`, `internal/worker/processor.go`, `internal/worker/attachstore.go`

**Module docs:** [modules/mail-worker/](../modules/mail-worker/index.md)

---

## mail-admin

**What it is:** GraphQL API for operational management. JWT-authenticated (HMAC-SHA256).

**Responsibilities:**
- Sender CRUD: create, update, delete sender configurations in NATS KV `senders`
- Read-only queries: audit log (`mails`), bounce records (`bounces`), dead letters (`deadLetters`) with filtering and pagination
- Mutation: `reprocessDeadLetter` — re-publish a payload to `DISPATCH_MAILS`
- Auth middleware: validates Bearer JWTs signed with `DISPATCH_ADMIN_AUTH_SECRET`

**Key files:** `cmd/mail-admin/main.go`, `internal/admin/resolver.go`, `internal/admin/auth.go`

**Module docs:** [modules/mail-admin/](../modules/mail-admin/index.md)

---

## bouncemanagement

**What it is:** Scheduled background job for NDR (non-delivery report) processing.

**Responsibilities:**
- Run every 15 minutes (and immediately on startup)
- Fetch unread NDR messages from a dedicated MS Graph bounce mailbox
- Extract `X-Dispatch-TraceId` from NDR body via regex
- Publish `BounceRecord` to `DISPATCH_BOUNCES` stream
- Mark processed messages as read via MS Graph PATCH

**Key files:** `cmd/bouncemanagement/main.go`, `internal/bounce/crawler.go`

**Module docs:** [modules/bounce-management/](../modules/bounce-management/index.md)

---

## msgraph (supporting layer)

**What it is:** MS Graph API client abstraction. Used by mail-worker and bouncemanagement.

**Responsibilities:**
- OAuth2 token acquisition with in-memory cache (60s buffer before expiry)
- HTTP client with circuit breaker (5 consecutive failures → open, 30s timeout), 3 retries with Retry-After backoff
- Email send: inline (≤3 MB attachments) or upload session (>3 MB, with chunked upload)
- Bounce mailbox polling: get unread messages, mark as read
- Per-sender rate limiting (token bucket: 1 req/s, burst 10)
- Error type discrimination: `GraphTransientError` (429/5xx/IO) vs `GraphPermanentError` (4xx≠429)
- Optional dev proxy mode (`MS_GRAPH_PROXY_URL`) with TLS verification disabled
- Optional mock token mode (`MS_GRAPH_MOCK_TOKEN`) for local dev without Azure credentials

**Key files:** `internal/msgraph/client.go`, `internal/msgraph/service.go`, `internal/msgraph/bounce.go`, `internal/msgraph/token.go`, `internal/msgraph/errors.go`, `internal/msgraph/ratelimiter.go`

**Module docs:** [modules/msgraph/](../modules/msgraph/index.md)

---

## core (foundational layer)

**What it is:** Domain types, configuration, and version. Everything in the system depends on this.

**Responsibilities:**
- Domain models: `MailRequest`, `MailRequestDO`, `AttachmentDO`, `Sender`, `AuditRecord`, `DeadLetter`, `BounceRecord`
- Error types: `ApiError`, `ValidationError`, `QuotaError`, `QuotaStateError`, `NatsPublishError` with typed `ErrorCode`
- Configuration: `Config` struct loaded from environment variables only (no config files)
- Version: build-time injection via ldflags

**Key files:** `internal/domain/mail.go`, `internal/domain/mail_request_do.go`, `internal/domain/sender.go`, `internal/domain/errors.go`, `internal/config/config.go`, `internal/version/version.go`

**Module docs:** [modules/core/](../modules/core/index.md)

---

## infrastructure (cross-cutting utilities)

**What it is:** Shared infrastructure packages with no domain logic.

**Responsibilities:**
- **loggy**: Structured JSON logger wrapping `slog` with 14 semantic event categories, API latency tracking, context-enriched loggers
- **natsutil**: NATS connection/initialization helpers, stream/KV/object store provisioning, subject/consumer name constants
- **hash**: SHA-256 fingerprint computation for spam deduplication
- **pii**: Email address masking for log safety (`user@domain.com` → `u***@domain.com`)

**Key files:** `internal/loggy/loggy.go`, `internal/natsutil/setup.go`, `internal/hash/hash.go`, `internal/pii/pii.go`

**Module docs:** [modules/infrastructure/](../modules/infrastructure/index.md)

---

## services (domain services backed by NATS KV)

**What it is:** Business logic services that operate on NATS KV buckets. Used by gateway and worker.

**Responsibilities:**
- **quota**: Rolling 24h recipient quota via NATS KV with optimistic CAS (max 10 retries), fail-closed
- **sender**: Sender configuration store with in-memory TTL cache (10 min) backed by NATS KV
- **spam**: Duplicate message detection using SHA-256 hashes in a NATS KV TTL bucket

**Key files:** `internal/quota/quota.go`, `internal/sender/sender.go`, `internal/spam/spam.go`

**Module docs:** [modules/services/](../modules/services/index.md)
