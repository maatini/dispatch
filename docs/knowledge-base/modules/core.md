# core

**Source:** `internal/domain/`, `internal/config/`, `internal/version/`

## Domain

| Type | Role |
|------|------|
| `MailRequest` | HTTP API model (base64 attach, validate tags) |
| `MailRequestDO` | NATS/worker model (`ObjectKey` serialized; `Content []byte` is `json:"-"`) |
| `Sender` | `DailyQuota <= 0` = unlimited; empty `AllowedDomains` = all allowed |
| Errors | `ValidationError`, `QuotaError`, `QuotaStateError`, `SpamStateError`, `NatsPublishError` — `ApiError` is JSON only, not `error` |

## Config

- Single `config.Load()` for all binaries; gateway auth enforced only in `cmd/mail-gateway`.
- Worker: `WorkerAckWait` / `WorkerMaxDeliver` via positive-int env helpers (defaults 5m / 8).
- Mock token path skips Graph credential validation.

## Version

- `version.Version` set via `-ldflags` at build; default string for local runs.
