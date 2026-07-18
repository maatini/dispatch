# services: Dependencies

## Depends On (Outbound)

| Service | Internal Deps | Go Modules | External |
|---|---|---|---|
| `quota` | `domain` (QuotaError, QuotaStateError) | `nats.go` (KV, JetStreamError) | NATS Server |
| `sender` | `domain` (Sender, ValidationError) | `nats.go` (KV) | NATS Server |
| `spam` | `domain` (ValidationError, SpamStateError, ErrorCode) | `nats.go` (KV Create) | NATS Server |

Note: `quota`, `sender`, and `spam` do **not** import `loggy`. They return typed domain errors; callers (gateway/admin) own logging.

## Used By

| Service | Used By | How |
|---|---|---|
| `quota.Checker` | `internal/gateway/` | `quotaChecker` interface → `Check()` |
| `sender.Store` | `internal/gateway/` | `senderLookup` interface → `Get()` |
| `sender.Store` | `internal/admin/` | Direct import → `Get()`, `Put()`, `Delete()`, `List()` |
| `spam.Checker` | `internal/gateway/` | `spamChecker` interface → `Check()` |

## Dependency Graph

```mermaid
graph TD
    subgraph Services
        QUOTA[quota.Checker]
        SENDER[sender.Store]
        SPAM[spam.Checker]
    end

    QUOTA --> DOMAIN[domain]
    QUOTA --> NATS[NATS Server]
    SENDER --> DOMAIN
    SENDER --> NATS
    SPAM --> DOMAIN
    SPAM --> NATS

    GATEWAY[gateway] --> QUOTA
    GATEWAY --> SENDER
    GATEWAY --> SPAM
    ADMIN[admin] --> SENDER
```
