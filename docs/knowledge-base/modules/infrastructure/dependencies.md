# infrastructure: Dependencies

## Depends On (Outbound)

| Package | Depends On | Type |
|---|---|---|
| `loggy` | `log/slog`, `sync`, `os`, `time` | Go stdlib only |
| `natsutil` | `github.com/nats-io/nats.go`, `time` | Go module + stdlib |
| `httpsrv` | `net/http`, `context`, `internal/loggy` | stdlib + loggy |
| `testkit` | `github.com/nats-io/nats.go`, `time` | Go module + stdlib (test-only) |
| `hash` | `crypto/sha256`, `fmt`, `strings` | Go stdlib only |
| `pii` | `strings` | Go stdlib only |

## Used By

| Consumer | Uses |
|---|---|
| `internal/gateway/` | loggy, natsutil, hash, pii |
| `internal/worker/` | loggy, natsutil, pii |
| `internal/admin/` | loggy, natsutil, pii |
| `internal/bounce/` | loggy, natsutil |
| `internal/msgraph/` | loggy |
| `cmd/mail-gateway`, `cmd/mail-admin` | loggy, natsutil, httpsrv |
| `cmd/mail-worker`, `cmd/bouncemanagement` | loggy, natsutil |
| unit tests (admin, quota, sender, spam, worker, …) | testkit |

Note: `quota`, `sender`, and `spam` do **not** import loggy; they return typed domain errors and leave logging to callers.

## Dependency Graph

```mermaid
graph TD
    subgraph Infrastructure
        LOGGY[loggy]
        NATSUTIL[natsutil]
        HTTPSRV[httpsrv]
        HASH[hash]
        PII[pii]
        TESTKIT[testkit]
    end

    LOGGY --> SLOG[log/slog stdlib]
    NATSUTIL --> NATSGO[nats.go]
    HTTPSRV --> LOGGY
    HTTPSRV --> NETHTTP[net/http]
    HASH --> SHA256[crypto/sha256]
    PII --> STRINGS[strings]
    TESTKIT --> NATSGO

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
    CMDS[cmd/*] --> NATSUTIL
    CMDS --> LOGGY
    CMDS --> HTTPSRV
```
