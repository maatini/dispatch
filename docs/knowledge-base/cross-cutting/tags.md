# Tags Registry

All `@tag:xxx` references used in this knowledge base, with one-line meanings.

| Tag | Meaning |
|---|---|
| `@tag:fail-closed` | Errors in quota/NATS/attachments → HTTP 503, never bypass checks |
| `@tag:nats-only-state` | NATS JetStream is the sole state backend — no PostgreSQL, Redis, etc. |
| `@tag:zero-double-delivery` | Idempotency via `delivered` KV bucket ensures each message is delivered at most once |
| `@tag:consumer-side-interfaces` | Go interfaces defined at point of use (consumer), not at definition (producer) |
| `@tag:exclusive-loggy` | All logging via `internal/loggy` — never `slog.*` or `fmt.Println` directly |
| `@tag:pii-mask-always` | All email addresses in logs must be masked via `pii.MaskEmail()` |
| `@tag:context-first` | `context.Context` as first parameter for all I/O/blocking functions; never stored in structs |
| `@tag:dedup` | Deduplication via SHA-256 spam hash (gateway) + traceID in `delivered` KV (worker) |
| `@tag:optimistic-cas` | Quota enforcement uses optimistic concurrency via NATS KV revision numbers |
| `@tag:circuit-breaker` | MS Graph calls protected by `sony/gobreaker` — 5 consecutive transient failures open the breaker |
| `@tag:test-mode` | `sender.Test == true` → worker skips MS Graph, writes `TEST_SUCCESS` audit |
| `@tag:work-queue` | `DISPATCH_MAILS` uses NATS WorkQueuePolicy — each message delivered to exactly one consumer |
| `@tag:streaming-attachments` | Gateway uploads attachments via streaming base64 decode — O(1) memory per attachment |
| `@tag:best-effort-cleanup` | Attachment cleanup in worker is best-effort; bucket TTL handles orphans |
| `@tag:jwt-hmac` | Admin API uses HMAC-SHA256 JWT with `DISPATCH_ADMIN_AUTH_SECRET` |
| `@tag:distroless` | Docker images use `gcr.io/distroless/static-debian12:nonroot` |
| `@tag:env-config` | All configuration via environment variables only — no config files |
