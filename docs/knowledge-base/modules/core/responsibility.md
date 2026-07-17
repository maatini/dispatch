# core: Responsibilities

## What It Owns

The core module is the **foundational layer** — it defines all domain types, value objects, error types, and system configuration. Every other package in the system imports at least one of these packages. Core itself has zero internal dependencies (only the Go standard library and NATS types for KV interfaces in error handling).

## Packages

### `internal/domain`

**Owns:** All domain models and error types shared across services.

| Model | Purpose | Key Fields |
|---|---|---|
| `MailRequest` | External API contract (gateway input) | `AppTag`, `Recipients[]`, `Subject`, `BodyContent`, `HtmlBodyContent`, `Attachments[]`, `TraceContext` |
| `MailRequestDO` | Internal/NATS representation | Adds `TraceID`, `Sender`, `Test`; attachments have `ObjectKey` instead of base64 `Content` |
| `AttachmentDO` | Attachment in internal representation | `ObjectKey` (gateway) or `Content` (worker, `json:"-"`) — never both |
| `Sender` | Sender configuration | `AppTag`, `Email`, `Test`, `DailyQuota` (0 = unlimited), `AllowedDomains` (empty = all allowed) |
| `AuditRecord` | Delivery outcome | `TraceID`, `Status` (DELIVERED/FAILED/TEST_SUCCESS), timestamp |
| `DeadLetter` | Unparseable message | `Payload` (raw string), `Error`, timestamp |
| `BounceRecord` | NDR bounce event | `OriginalTraceID`, `BounceReason`, timestamp |

**Error types:**

| Type | Purpose | Implements |
|---|---|---|
| `ApiError` | HTTP response body for all errors | `Status`, `Code` (ErrorCode), `Message`, `TraceID` |
| `ValidationError` | Input validation failures (400) | `Code` + `Message` |
| `QuotaError` | Quota exceeded (429) | `Limit`, `Current`, `Requested` |
| `QuotaStateError` | Quota KV unavailable (503, fail-closed) | Wraps cause via `Unwrap()` |
| `NatsPublishError` | NATS publish failure (503) | Wraps cause via `Unwrap()` |

### `internal/config`

**Owns:** System configuration loaded from environment variables.

- Single `Config` struct shared across all 4 binaries
- `Load()` reads from env vars with defaults
- No config files — environment-only
- Required: `NATS_URL`, MS Graph credentials (unless mock token), `DISPATCH_ADMIN_AUTH_SECRET`

### `internal/version`

**Owns:** Build version string.

- `var Version = "0.5.0"` set at build time via `-ldflags`

## Invariants

| Invariant | Enforcement |
|---|---|
| `DailyQuota` 0 or negative = unlimited | `quota.Checker.Check()` returns nil when `limit <= 0` |
| `AllowedDomains` empty = all domains allowed | `checkDomains()` returns nil when `sender.AllowedDomains == ""` |
| `MailRequestDO.AttachmentDO.Content` is never serialized | `json:"-"` tag |
| Config validation happens at startup | `config.Load()` returns error for missing required fields |
| Error codes are typed constants | `type ErrorCode string` with named constants |

## What It Does NOT Own

- Behavior/logic — validation logic is in `gateway`, delivery logic is in `worker`, quota logic is in `quota`
- Persistence — domain types are plain structs; NATS interaction is in other packages
