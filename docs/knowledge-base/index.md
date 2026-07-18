# Dispatch Knowledge Base

LLM-optimized architecture notes for multi-tenant email delivery (Go + NATS JetStream + MS Graph).

**Canonical pipeline diagrams & error table:** root [`ARCHITECTURE.md`](../../ARCHITECTURE.md)  
**AI change log:** [`docs/ai-changes.md`](../ai-changes.md)

## Quick navigation

| I need to... | Go to... |
|---|---|
| Purpose & tech stack | [overview.md](overview.md) |
| Architectural decisions (ADRs) | [decisions.md](decisions.md) |
| HTTP send pipeline | [modules/mail-gateway.md](modules/mail-gateway.md) |
| Worker / ACK / dedup | [modules/mail-worker.md](modules/mail-worker.md) |
| GraphQL admin | [modules/mail-admin.md](modules/mail-admin.md) |
| NDR bounce crawler | [modules/bounce-management.md](modules/bounce-management.md) |
| MS Graph client | [modules/msgraph.md](modules/msgraph.md) |
| Domain / config | [modules/core.md](modules/core.md) |
| loggy / natsutil / httpsrv | [modules/infrastructure.md](modules/infrastructure.md) |
| quota / sender / spam | [modules/services.md](modules/services.md) |
| @tag registry | [cross-cutting/tags.md](cross-cutting/tags.md) |
| Shared Go patterns | [cross-cutting/shared-patterns.md](cross-cutting/shared-patterns.md) |
| How to maintain this KB | [maintenance.md](maintenance.md) |

## Structure

```
docs/knowledge-base/
├── index.md
├── overview.md
├── decisions.md              # stable ADRs
├── modules/                  # one file per module (responsibility + gotchas)
├── cross-cutting/
└── maintenance.md
```

Each module file is intentionally short: ownership, entry points, and gotchas only. Dependencies and sequence diagrams live in `ARCHITECTURE.md` to avoid duplication.
