# core: Dependencies

## Depends On (Outbound)

| Dependency | Type | Purpose |
|---|---|---|
| `encoding/json` | Go stdlib | Struct tags |
| `fmt` | Go stdlib | Error formatting |
| `os`, `strconv`, `strings` | Go stdlib | Env var loading in config |
| `time` | Go stdlib | Duration type in config |
| `github.com/go-playground/validator/v10` | Go module | Struct validation tags (indirect — only in import for tags) |

## Used By (Everything depends on core)

| Consumer | Uses |
|---|---|
| `internal/gateway/` | `MailRequest`, `MailRequestDO`, `AttachmentDO`, `Sender`, `ApiError`, `ValidationError`, `QuotaError`, `QuotaStateError`, `NatsPublishError` |
| `internal/worker/` | `MailRequestDO`, `AttachmentDO`, `AuditRecord`, `DeadLetter` |
| `internal/admin/` | `Sender`, `AuditRecord`, `BounceRecord`, `DeadLetter` |
| `internal/bounce/` | `BounceRecord` |
| `internal/msgraph/` | `MailRequestDO`, `AttachmentDO` |
| `internal/quota/` | `QuotaError`, `QuotaStateError` |
| `internal/sender/` | `Sender`, `ValidationError` |
| `internal/spam/` | `ValidationError` |
| `cmd/*/main.go` | `Config`, `Version` |

## Dependency Graph

```mermaid
graph TD
    subgraph Core
        DOMAIN[domain]
        CONFIG[config]
        VERSION[version]
    end

    DOMAIN --> STDJSON[encoding/json]
    DOMAIN --> STDFMT[fmt]
    CONFIG --> STDOS[os/strconv/strings/time]
    CONFIG --> DOMAIN
    VERSION --> NOTHING[nothing]

    GATEWAY[gateway] --> DOMAIN
    WORKER[worker] --> DOMAIN
    ADMIN[admin] --> DOMAIN
    BOUNCE[bounce] --> DOMAIN
    MSGRAPH[msgraph] --> DOMAIN
    QUOTA[quota] --> DOMAIN
    SENDER[sender] --> DOMAIN
    SPAM[spam] --> DOMAIN

    GATEWAY --> CONFIG
    WORKER --> CONFIG
    ADMIN --> CONFIG
    BOUNCE --> CONFIG
```
