# CLAUDE-go.md

Behavioral guidelines for the CodyMail Go implementation.
Supercedes CLAUDE.md for this codebase.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

Nutze bevorzugt die Devbox-Umgebung.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior Go engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it — don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure `devbox run test` passes before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

---

## Project: codymail-go

Multi-tenant email delivery system. REST input → NATS JetStream → MS Graph API.
Production-grade, deployed on Kubernetes (AKS).

### Modules

| Command | Purpose |
|---------|---------|
| `cmd/mail-gateway` | HTTP REST entry point (`POST /codymail/api/v1/mail/send`) |
| `cmd/mail-worker` | NATS JetStream consumer → MS Graph email delivery |
| `cmd/mail-admin` | GraphQL API for tenant/sender management and audit logs |
| `cmd/bouncemanagement` | Scheduled NDR/bounce crawler via MS Graph |
| `internal/` | Shared domain models, NATS services, MS Graph client |

### Stack

- **Go 1.24** — single static binary per service
- **NATS JetStream** — message broker; subject: `cody.mailing.job.request.mails`
- **NATS JetStream KV** — persistent state backend (quota, spam cache, sender config)
- **NATS JetStream Streams** — append-only audit log, dead-letter store, bounce records
- **MS Graph API v1.0** — email delivery via Microsoft 365
- **`log/slog`** — structured JSON logging
- **`github.com/go-chi/chi/v5`** — HTTP routing
- **`github.com/go-playground/validator/v10`** — request validation
- **`github.com/nats-io/nats.go`** — NATS client (JetStream, KV, ObjectStore)
- **`github.com/sony/gobreaker`** — circuit breaker for MS Graph calls
- **`golang.org/x/time/rate`** — token-bucket rate limiter per sender

**Kein PostgreSQL. Kein externes State-Backend außer NATS.**

### Architecture

```
NATS KV Buckets (State):
  senders   → Sender-Konfiguration (appTag → Email, Quota, AllowedDomains)
  quota     → Rolling-24h-Verbrauch pro appTag (optimistic CAS)
  spam      → SHA-256-Hashes, TTL 60s

NATS Streams (Events):
  DISPATCH_MAILS        → Broker-Payload (Work Queue)
  DISPATCH_AUDIT        → Delivery-Ergebnisse (DELIVERED / FAILED)
  DISPATCH_DEAD_LETTERS → nicht-parsbare Nachrichten
  DISPATCH_BOUNCES      → NDR-Ergebnisse aus Bounce Crawler

REST (Gateway) → 5-Stage Validation (Format / Sender-Lookup / Domain / Quota / Spam)
    → NATS JetStream publish → DISPATCH_MAILS
        → Fehler: HTTP 503 (kein Fallback, kein Retry im Gateway)
        → Erfolg: HTTP 202
    → Worker (NATS Consumer, explicit ACK)
    → MS Graph API
    → NATS DISPATCH_AUDIT (DELIVERED / FAILED)
```

**Resilience:**
- NATS unreachable at publish time → HTTP 503 immediately, no silent retry
- Quota KV error → fail-closed (HTTP 503), never bypass
- MS Graph 429/5xx → no NATS ACK, JetStream redelivers; no audit entry written
- MS Graph 4xx → ACK (poison pill), append FAILED to DISPATCH_AUDIT
- Malformed JSON → ACK, append to DISPATCH_DEAD_LETTERS
- Worker crash after Graph success, before ACK → idempotent dedup via DISPATCH_AUDIT lookup

### Key Domain Concepts

- **appTag**: Tenant identifier (e.g. `alv-dev`). Each sender has exactly one appTag.
- **MailRequestDO**: Enriched NATS payload (≠ REST struct `MailRequest`)
- **Sender**: Tenant config — stored in NATS KV bucket `senders`; maps appTag to technical sender email, daily quota, allowed domains. In-memory cached in gateway (TTL 10 min).
- **Quota**: Rolling 24h window, counts recipients (TO+CC+BCC). State in NATS KV bucket `quota` per appTag, updated via optimistic CAS (`nats.KeyValue.Update`). Fail-closed: any KV error → HTTP 503, never bypass.
- **Spam cache**: SHA-256 fingerprint over (appTag, subject, recipients, body lengths). Stored in NATS KV bucket `spam` with bucket-level TTL of 60s. Per-pod in-memory fallback acceptable (minor false-negative risk in multi-replica deployments).
- **Bounce matching**: 3-tier — trace ID in NDR body → attachment scan → recipient lookup in DISPATCH_AUDIT stream

**Error types** (all in `internal/domain/errors.go`):

| Type | HTTP | NATS ACK | NATS write |
|------|------|----------|------------|
| `ValidationError` | 400 | — | — |
| `QuotaError` | 429 | — | — |
| `NatsPublishError` | 503 | — | — |
| `QuotaStateError` (fail-closed) | 503 | — | — |
| `GraphTransientError` (429/5xx) | — | No | — |
| `GraphPermanentError` (4xx) | — | Yes | `DISPATCH_AUDIT` (FAILED) |
| `ParseError` | — | Yes | `DISPATCH_DEAD_LETTERS` |

### Naming Conventions

Follow standard Go idioms. Deviations require justification.

- Structs: `MailRequest` (REST DTO), `MailRequestDO` (broker payload — `DO` suffix kept for cross-system traceability)
- Interfaces: single-method interfaces named after the method (`type Sender interface { Send(...) }`)
- Errors: lowercase sentinel values (`var ErrQuotaExceeded = errors.New(...)`) or typed errors with `Error()` method
- Files: one primary type per file, filename matches type in snake_case (`mail_request_do.go`)
- Tests: same package as subject (`package gateway`) for white-box; `_test` suffix package for black-box
- Integration tests: `//go:build integration` tag, file suffix `_integration_test.go`
- No `util`, `helper`, `common` package names — use domain-specific names

### Go Idioms — Enforce These

**Errors:**
```go
// Correct: wrap with context, check with errors.Is / errors.As
if err != nil {
    return fmt.Errorf("quota check for %s: %w", appTag, err)
}

// Wrong: discard or log-and-return
_ = err
log.Println(err); return nil
```

**Interfaces:**
```go
// Define interfaces at the point of use (consumer), not at the point of definition (producer).
// internal/gateway/quota.go defines:
type QuotaChecker interface {
    Check(ctx context.Context, appTag string, count int) (QuotaResult, error)
}
// NOT in internal/quota/service.go
```

**Goroutines:**
```go
// Always pair goroutines with cancellation and error propagation.
// Use errgroup for concurrent work with shared cancellation.
g, ctx := errgroup.WithContext(ctx)
g.Go(func() error { return worker.Run(ctx) })
if err := g.Wait(); err != nil { ... }
```

**Context:**
```go
// First parameter of every function that does I/O or blocking work.
func (s *GraphService) SendEmail(ctx context.Context, req MailRequestDO) error
// Never store context in a struct.
```

**Logging:**
```go
// Use slog with structured fields. Never fmt.Println or log.Printf in production code.
slog.InfoContext(ctx, "mail dispatched to NATS",
    slog.String("traceId", req.TraceId),
    slog.String("appTag", req.AppTag),
)
// PII: always mask before logging
slog.WarnContext(ctx, "domain not whitelisted",
    slog.String("recipient", pii.MaskEmail(addr)),
)
```

### Build & Test

**Immer Devbox verwenden** — sie stellt Go 1.24 und golangci-lint bereit.

```bash
devbox shell                    # Umgebung aktivieren

devbox run build                # go build ./...
devbox run test                 # go test ./...
devbox run test-gateway         # nur Gateway + interne Gateway-Packages
devbox run test-worker          # nur Worker + interne Worker-Packages
devbox run test-admin           # nur Admin
devbox run test-bounce          # nur Bounce Management
devbox run test-integration     # Integrationstests mit Testcontainers (Docker nötig)
devbox run lint                 # golangci-lint run ./...
devbox run generate             # go generate ./... (gqlgen, mockgen)
devbox run coverage             # Tests + ASCII Coverage-Report
devbox run up                   # NATS via Docker Compose starten
devbox run down                 # Docker Compose stoppen
```

Innerhalb einer aktiven `devbox shell` kann auch direkt `go` aufgerufen werden:

```bash
go test ./internal/gateway/...                              # wie devbox run test-gateway
go test -tags integration -run TestNATSConsumer ./...       # einzelner Integrationstest
go test -run TestQuotaService -v ./internal/gateway/        # mit Output
```

- Unit-Tests: keine externen Abhängigkeiten, in-process Mocks via Interfaces
- Integrationstests (`//go:build integration`): Testcontainers-go für NATS, Docker muss laufen
- CI führt nur Unit-Tests aus (`go test ./...` ohne `-tags integration`)
- Coverage-Ziel: 80% Statement Coverage

### Configuration

Alle Werte kommen aus Umgebungsvariablen. Kein Konfigurationsformat (YAML, TOML, properties).
Defaults werden im Code definiert, nie außerhalb.

**Pflicht (kein Default — Service startet nicht ohne diese):**
```
NATS_URL
MS_GRAPH_TENANT_ID
MS_GRAPH_CLIENT_ID
MS_GRAPH_CLIENT_SECRET
MS_GRAPH_SENDER_EMAIL
```

**Optional (mit Defaults):**
```
PORT=8080
DISPATCH_SPAM_TIMEOUT_SECONDS=60
DISPATCH_VALIDATION_MAX_BODY_SIZE=10000000
DISPATCH_VALIDATION_MIME_WHITELIST=application/pdf,image/jpeg,...
DISPATCH_MAX_TOTAL_ATTACHMENT_SIZE_MB=20
DISPATCH_NATS_PUBLISH_TIMEOUT_SECONDS=5
DISPATCH_GRAPH_RATE_LIMITER_SKIP_SLEEP=false
```

Config-Struct in `internal/config/config.go` mit explizitem `Load() (Config, error)`.
Fehlende Pflichtfelder → sofortiger Startabbruch mit klarer Fehlermeldung.

### What NOT to do

- **Quota-Check niemals bypassen** — fail-closed ist intentional; jeder KV-Fehler → HTTP 503
- **`GraphTransientError` nie in NATS schreiben** — kein ACK, kein Audit-Eintrag; JetStream redelivert
- **NATS-Publish-Fehler nie swallowed** — immer als HTTP 503 zurückgeben, nie loggen-und-202
- **Keine E-Mail-Adressen in Logs** — immer `pii.MaskEmail()` verwenden
- **Kein PostgreSQL, kein `database/sql`, kein ORM** — einziges State-Backend ist NATS (KV + Streams)
- **Kein State-Backend außer NATS einführen** — kein Redis, kein SQLite, keine eingebettete DB
- **NATS KV nie als direkte Cache-Schicht missbrauchen** — KV ist Source of Truth, nicht Leseoptimierung; In-Memory-Cache darüber ist erlaubt und erwünscht (Sender-Config-Cache)
- **Kein globaler State** — keine `var`-Level Singletons außer gecachten, immutablen Werten
- **Keine `init()`-Funktionen** — Initialisierung explizit in `main.go`
- **Kein `context.Background()` tief im Call-Stack** — Context immer von oben propagieren
- **Fehler nie mit `_` ignorieren** — jeden `error`-Rückgabewert behandeln oder explizit propagieren

---

## Quality Gate (ersetzt SonarQube)

Nach jeder Code-Änderung:

```bash
devbox run lint    # golangci-lint — muss ohne Findings durchlaufen
devbox run test    # alle Unit-Tests — müssen grün sein
```

Maximal 3 Fix-Zyklen: Lint → Fix → Re-Lint.
Bei Lint-Findings: alle auflisten, alle beheben — kein `//nolint` ohne Begründungskommentar.

**Aktive Linter** (via `.golangci.yml`):
- `errcheck` — keine ignorierten errors
- `govet` — `go vet`-Checks
- `staticcheck` — tiefere statische Analyse
- `exhaustive` — Switch-Statements auf Enums vollständig
- `revive` — idiomatisches Go
- `goimports` — Import-Sortierung
- `misspell` — Tippfehler in Kommentaren/Strings

## Workflow

1. Code schreiben
2. `devbox run lint` — alle Findings beheben
3. `devbox run test` — alle Unit-Tests grün
4. Erst dann als fertig markieren


