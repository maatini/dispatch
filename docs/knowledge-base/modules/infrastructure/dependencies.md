# infrastructure: Dependencies

## Depends On (Outbound)

| Package | Depends On | Type |
|---|---|---|
| `loggy` | `log/slog`, `sync`, `os`, `time` | Go stdlib only |
| `natsutil` | `github.com/nats-io/nats.go`, `time` | Go module + stdlib |
| `hash` | `crypto/sha256`, `fmt`, `strings` | Go stdlib only |
| `pii` | `strings` | Go stdlib only |

## Used By (Everything)

| Consumer | Uses |
|---|---|
| `internal/gateway/` | loggy, natsutil, hash, pii |
| `internal/worker/` | loggy, natsutil, pii |
| `internal/admin/` | loggy, natsutil, pii |
| `internal/bounce/` | loggy, natsutil |
| `internal/msgraph/` | loggy |
| `internal/quota/` | loggy |
| `internal/sender/` | loggy |
| `internal/spam/` | loggy |
| `cmd/*/main.go` | loggy, natsutil |

## Dependency Graph

```mermaid
graph TD
    subgraph Infrastructure
        LOGGY[loggy]
        NATSUTIL[natsutil]
        HASH[hash]
        PII[pii]
    end

    LOGGY --> SLOG[log/slog stdlib]
    NATSUTIL --> NATSGO[nats.go]
    HASH --> SHA256[crypto/sha256]
    PII --> STRINGS[strings]

    GATEWAY[gateway] --> LOGGY
    GATEWAY --> NATSUTIL
    GATEWAY --> HASH
    GATEWAY --> PII
    WORKER[worker] --> LOGGY
    WORKER --> NATSUTIL
    WORKER --> PII
    ADMIN[admin] --> LOGGY
    ADMIN --> NATSUTIL
    ADMIN --> PII
    BOUNCE[bounce] --> LOGGY
    BOUNCE --> NATSUTIL
    MSGRAPH[msgraph] --> LOGGY
    QUOTA[quota] --> LOGGY
    SENDER[sender] --> LOGGY
    SPAM[spam] --> LOGGY
```
