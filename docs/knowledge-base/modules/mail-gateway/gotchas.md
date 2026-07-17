# mail-gateway: Gotchas

## Pipeline Ordering Is Critical

The 7-stage pipeline order is intentional. Do not reorder stages:
- Sender lookup (2) must come before domain whitelist (3) — need `AllowedDomains` from sender config
- Domain whitelist (3) must come before quota (4) — don't count rejected recipients toward quota
- Quota (4) before spam (5) — quota is a hard limit, spam is a soft duplicate check
- Spam (5) before attachment upload (6) — don't store attachments for duplicates
- Attachment upload (6) before NATS publish (7) — don't publish if Object Store is unavailable

## Fail-Closed Means No Fallback

When `quota.Checker.Check()` returns `QuotaStateError` (any KV error), the gateway returns HTTP 503. There is no "degraded mode" or bypass. This is by design — see [architecture/decisions.md](../../architecture/decisions.md).

## Attachment Upload Is Streaming

The gateway never holds full attachment bytes in memory. `gateway.attachstore.go` uses `base64.NewDecoder(strings.NewReader(att.Content))` and streams directly to `nats.ObjectStore.Put()`. This is O(1) memory per attachment regardless of size.

## Response Always Includes traceId

`traceID` is generated at the top of `handleSend` via `uuid.New()` and included in every response (success and error). This is the correlation ID used throughout the system.

## MaxBytesReader Catches Oversized Bodies

`http.MaxBytesReader` is applied before JSON decoding. If the body exceeds the limit, the JSON decoder sees a truncated stream and returns a `MaxBytesError`, which is mapped to HTTP 413. The error type check uses `errors.As(err, &maxBytesErr)`.

## NatsPublishError Wraps the Cause

When publishing fails, the error is wrapped in `domain.NatsPublishError{Cause: err}`. The handler checks for this type and returns HTTP 503. Any other unexpected error returns HTTP 500 with code `INTERNAL_ERROR` and a generic `"internal error"` message — internal error details never leak to clients.

## Error Contract: Codes Are Never Empty

Every error response carries a typed `code`:
- Struct validation failures → 400 `VALIDATION_FAILED` (not `UNKNOWN_APP_TAG` — that code is only for unknown appTags from the sender lookup)
- `SpamStateError` (spam KV failure) → 503, same as `QuotaStateError` — fail-closed state errors are one class
- Non-domain errors → 500 `INTERNAL_ERROR`

## Health Endpoints: Readiness Is Real, Liveness Is Trivial

- `/health` and `/health/ready` check `nc.Status()`: anything but `nats.CONNECTED` → HTTP 503 with `"status": "DOWN"`. Broken pods leave the rotation.
- `/health/live` always returns 200 — it only signals the process is alive.
