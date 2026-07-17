# Overview

**Dispatch** is a multi-tenant email delivery system. It accepts mail send requests via a REST API, routes them through NATS JetStream, and delivers them via the Microsoft Graph API.

## Purpose

Reliable, scalable, multi-tenant email delivery with:
- Strict quota, spam, and deduplication enforcement (fail-closed philosophy)
- Zero double-delivery guarantee
- NATS JetStream as the sole state backend (no PostgreSQL, Redis, or external database)

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.25+ (toolchain go1.25.9) |
| HTTP Router | `github.com/go-chi/chi/v5` |
| Message Broker & State | **NATS JetStream** — streams, KV store, object store |
| Email Transport | **Microsoft Graph API v1.0** |
| Validation | `github.com/go-playground/validator/v10` |
| Admin API | `github.com/graph-gophers/graphql-go` (GraphQL) |
| Auth | `github.com/golang-jwt/jwt/v5` (HMAC-SHA256) |
| Resilience | `github.com/sony/gobreaker` (circuit breaker), `golang.org/x/time/rate` (token bucket) |
| Container | Distroless (`gcr.io/distroless/static-debian12:nonroot`) |
| Dev Env | [Devbox](https://www.jetpack.io/devbox) |
| CI/CD | GitHub Actions (build, test, Trivy scan, SBOM, GHCR push) |

## Services (4 deployable binaries)

| Service | Entry Point | Responsibility |
|---|---|---|
| `mail-gateway` | `POST /dispatch/api/v1/mail/send` | HTTP ingress: 7-stage validation pipeline → NATS publish |
| `mail-worker` | NATS pull consumer `mail-worker` | Deserialize → dedup → MS Graph delivery → audit |
| `mail-admin` | `POST /graphql` | Authenticated GraphQL API: sender CRUD, audit/bounce/dead-letter queries, reprocessing |
| `bouncemanagement` | Ticker (every 15 min) | NDR crawler: poll MS Graph bounce mailbox → extract trace IDs → publish bounce records |

## Key Design Principles

- **Fail-closed (@tag:fail-closed)**: Quota/NATS/attachment errors → HTTP 503, never bypass checks
- **NATS-only state (@tag:nats-only-state)**: All persistent state in NATS KV, streams, and object store — no external databases
- **Zero double-delivery (@tag:zero-double-delivery)**: Idempotency via NATS KV `delivered` bucket (7-day TTL)
- **Consumer-side interfaces (@tag:consumer-side-interfaces)**: Go interfaces defined at point of use, never at definition
- **Exclusive logging (@tag:exclusive-loggy)**: All logs via `internal/loggy` — never `slog.*` or `fmt.Println` directly
- **PII masking (@tag:pii-mask-always)**: All email addresses in logs masked via `pii.MaskEmail()`
- **Context-first params (@tag:context-first)**: `context.Context` as first parameter for all I/O/blocking functions; never stored in structs

See [architecture/decisions.md](architecture/decisions.md) for the rationale behind these choices.
