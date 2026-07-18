# mail-gateway

**Source:** `cmd/mail-gateway/`, `internal/gateway/`  
**Role:** HTTP ingress — validate send requests, enforce quota/spam, publish to JetStream.

## Entry points

| Route | Auth | Purpose |
|-------|------|---------|
| `POST /dispatch/api/v1/mail/send` | Bearer `DISPATCH_GATEWAY_AUTH_TOKEN` | Accept mail |
| `GET /health`, `/health/ready` | none | Real readiness (`nc.Status` == CONNECTED) |
| `GET /health/live` | none | Always 200 |

## Pipeline (order is fixed — do not reorder)

1. JSON + validation → 2. Sender lookup → 3. Domain whitelist → 4. Quota → 5. Spam (`spam.Hash` + KV) → 6. Attachment Object Store → 7. NATS publish → 202

Failures: validation 400/413; quota exceeded 429; state/publish/attach 503; unauthorized 401. Full diagram: root `ARCHITECTURE.md`.

## Gotchas

- **AuthN is cluster-wide** — not per-`appTag` AuthZ. Token enforced only in `cmd/mail-gateway` (shared `config.Load` does not require it). Disable only via `DISPATCH_GATEWAY_AUTH_DISABLED=true` (local).
- **Fail-closed** — `QuotaStateError` / `SpamStateError` / attach / publish → 503, no bypass.
- **Attachments stream** base64 → Object Store (O(1) memory); never publish without successful upload.
- **Every response includes `traceId`** (UUID at start of `handleSend`).
- **`MaxBytesReader` before decode** → oversized body → 413 via `MaxBytesError`.
- Domain whitelist before quota (rejected recipients must not count). Spam after quota; attach after spam.
