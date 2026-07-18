# Dispatch Knowledge Base

LLM-optimized architecture documentation for [dispatch](https://github.com/maatini/dispatch) — a multi-tenant email delivery system (Go + NATS JetStream + MS Graph API).

**Start here when you need to understand any part of the system without reading the entire codebase.**

## Quick Navigation

| I need to... | Go to... |
|---|---|
| Understand the project purpose & tech stack | [overview.md](overview.md) |
| See the big-picture architecture & component map | [architecture/index.md](architecture/index.md) |
| Understand who depends on what | [architecture/dependencies.md](architecture/dependencies.md) |
| Follow a key data flow (send mail, bounce detection) | [architecture/data-flows.md](architecture/data-flows.md) |
| Read key architectural decisions | [architecture/decisions.md](architecture/decisions.md) |
| Work on the HTTP ingress / 7-stage pipeline | [modules/mail-gateway/index.md](modules/mail-gateway/index.md) |
| Work on the NATS consumer / email delivery | [modules/mail-worker/index.md](modules/mail-worker/index.md) |
| Work on the GraphQL admin API | [modules/mail-admin/index.md](modules/mail-admin/index.md) |
| Work on the NDR bounce crawler | [modules/bounce-management/index.md](modules/bounce-management/index.md) |
| Understand the MS Graph integration | [modules/msgraph/index.md](modules/msgraph/index.md) |
| Work on domain types, config, or version | [modules/core/index.md](modules/core/index.md) |
| Work on logging, NATS setup, HTTP lifecycle, hashing, or PII masking | [modules/infrastructure/index.md](modules/infrastructure/index.md) |
| Work on quota, sender cache, or spam dedup | [modules/services/index.md](modules/services/index.md) |
| Find cross-cutting @tag definitions | [cross-cutting/tags.md](cross-cutting/tags.md) |
| Follow coding conventions used across modules | [cross-cutting/shared-patterns.md](cross-cutting/shared-patterns.md) |
| Update this knowledge base | [maintenance.md](maintenance.md) |

## Structure

```
docs/knowledge-base/
├── index.md                    ← you are here
├── overview.md                 # High-level project purpose, tech stack, key principles
├── architecture/               # Global architecture: components, dependencies, data flows, decisions
├── modules/                    # One folder per major logical component (8 modules)
│   ├── mail-gateway/           # HTTP ingress + 7-stage validation pipeline
│   ├── mail-worker/            # NATS pull consumer → MS Graph delivery
│   ├── mail-admin/             # GraphQL API for sender CRUD + audit queries
│   ├── bounce-management/      # Scheduled NDR crawler
│   ├── msgraph/                # MS Graph API client (auth, circuit breaker, send, bounce polling)
│   ├── core/                   # Domain types, config loading, version
│   ├── infrastructure/         # loggy, natsutil, httpsrv, testkit, hash, pii
│   └── services/               # quota, sender store, spam dedup
├── cross-cutting/              # Tags registry, shared patterns
└── maintenance.md              # How to keep this KB up to date
```

Each module folder contains:
- `index.md` — summary + links to detailed files
- `responsibility.md` — what this module owns, invariants, entry points
- `dependencies.md` — inbound/outbound dependencies with tables + diagrams
- `interfaces.md` — public API surface (HTTP, NATS subjects, function signatures, data structures)
- `gotchas.md` — pitfalls, ordering constraints, error semantics to remember

## Key Pointer Files

- `CLAUDE.md` at the repo root — AI behavioral guidelines + coding conventions + invariants
- `ARCHITECTURE.md` at the repo root — detailed architecture with diagrams (German language)
- `README.md` at the repo root — user-facing overview + API docs + configuration
- `docs/ai-changes.md` — mandatory AI change log (German language, max 5 lines per entry)
