# CLAUDE.md – dispatch (Multi-Tenant Email Delivery)

**Ziel (WHY):** Zuverlässige, skalierbare, mandantenfähige E-Mail-Zustellung über MS Graph mit strikten Quota-, Spam- und Deduplikationsregeln. Zero Double-Delivery. Fail-Closed Philosophie.

**Wichtige Commands (immer devbox nutzen):**
```bash
devbox run build && devbox run test && devbox run lint
devbox run up          # NATS + optional MS Graph Proxy
devbox run down
```

## 1. Behavioral Guidelines (Core – immer befolgen)

**Tradeoff:** Caution over speed. For trivial tasks use judgment.

- **Think Before Coding**: State assumptions explicitly. Present alternatives. Ask if unclear. Never hide confusion.
- **Simplicity First**: Minimum code that solves the problem. No speculative features, abstractions or configurability.
- **Surgical Changes**: Touch only what is required. Match existing style. Remove only your own orphans. Never refactor unrelated code.
- **Goal-Driven Execution**: Turn every task into verifiable goals with explicit success criteria. For multi-step tasks state a brief plan with verification steps.

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation.

## 2. Tech Stack & Environment

- **Go 1.25** – single static binary per service
- **NATS JetStream** – sole state backend (KV + Streams + Object Store)
- **MS Graph API v1.0** – email delivery
- `internal/loggy` – exclusive logging wrapper (never use `slog.*` or `fmt.Println` directly)
- **Always** activate devbox environment before any build/test/lint command

## 3. Project Structure (High-Level)

```
cmd/
  mail-gateway/     # REST entrypoint (POST /dispatch/api/v1/mail/send)
  mail-worker/      # NATS Pull-Consumer → MS Graph delivery
  mail-admin/       # GraphQL tenant/sender/audit management
  bouncemanagement/ # Scheduled NDR crawler
internal/           # domain models, services, clients (incl. loggy, natsutil, httpsrv, testkit)
docs/
  ai-changes.md     # AI change log
  knowledge-base/   # architecture, modules, gotchas
ARCHITECTURE.md     # detailed pipeline (German)
README.md           # user-facing overview + API
```

**Read when:** Detailed pipeline, error types or resilience logic → `ARCHITECTURE.md` and `docs/knowledge-base/`

## 4. Core Domain Invariants (MUST)

- `appTag` = unique Tenant identifier
- **Quota**: rolling 24h window (TO+CC+BCC), optimistic CAS via NATS KV, **fail-closed** (any error → HTTP 503)
- **Spam cache**: SHA-256 fingerprint, bucket TTL 60s
- **Delivered dedup**: 7-day TTL in NATS KV; Get **and** Put fail-closed (Put fail → no ACK)
- **Gateway AuthN**: Bearer `DISPATCH_GATEWAY_AUTH_TOKEN` on `/mail/send` (health open; disable only via `DISPATCH_GATEWAY_AUTH_DISABLED` for local)
- **Never** bypass quota check or deduplication

## 5. Coding Conventions & Go Idioms

- Errors: always wrap with context (`fmt.Errorf("...: %w", err)`), use `errors.Is` / `errors.As`
- Interfaces: define at point of **use** (consumer), never at definition (producer)
- Context: first parameter for every I/O or blocking function. Never store in structs.
- Logging: **exclusively** `loggy.GetLogger("ComponentName")` + semantic methods (`Info`, `Warnc`, `Critical`, `RecordApiStart` etc.)
- PII: always mask with `pii.MaskEmail(addr)`
- No `init()` functions, no global mutable state, no `context.Background()` deep in call stack

## 6. Error Handling & Resilience

- Quota / NATS publish / Attachment upload errors → HTTP 503 (fail-closed)
- MS Graph 429/5xx → **no ACK** (JetStream redelivers, respect Retry-After ≤ 30s)
- MS Graph 4xx → ACK + FAILED entry in `DISPATCH_AUDIT`
- Malformed JSON → ACK + entry in `DISPATCH_DEAD_LETTERS`

**Read when:** Working on worker or gateway error paths → `ARCHITECTURE.md` and `docs/knowledge-base/modules/mail-worker/` / `mail-gateway/`

## 7. What NOT to Do

- **Never** bypass quota check (fail-closed is intentional)
- **Never** swallow NATS publish errors (always return 503)
- **Never** call `slog.*` or `fmt.Println` in production code
- **Never** store context in structs
- **Never** introduce new state backends (no PostgreSQL, Redis, SQLite, etc.)
- **Never** delete pre-existing dead code unless explicitly asked
- **Never** ignore error return values (`_ = err`)

## 8. Workflow & Qualitätssicherung

**After every non-trivial change** (more than pure formatting or typos):

1. Run `devbox run lint && devbox run test` and verify both pass.
2. Append a short entry to `docs/ai-changes.md` (strict format defined in that file – max. 5 lines).
3. For complex tasks (> 3 steps or any uncertainty): **propose a plan first** and wait for confirmation.

**At the end of longer sessions:**
- Read the current `docs/ai-changes.md` and suggest which new insights or gotchas should be added to `CLAUDE.md`, `ARCHITECTURE.md`, or `docs/knowledge-base/`.

Mark important design decisions in the log with `**WICHTIG**` or `**DESIGN-DECISION**`.

## 9. References & Further Reading

- `docs/ai-changes.md` – complete AI change log with justifications
- `ARCHITECTURE.md` – full 7-stage pipeline, error table, resilience details
- `docs/knowledge-base/` – module responsibilities, interfaces, gotchas, shared patterns
- `docs/knowledge-base/cross-cutting/shared-patterns.md` – logging, errors, interfaces idioms

**Last updated:** 2026-07-18 | Version: 2.3 (P0 invariants + stale plan cleanup)

## 10. Knowledge Base

For architecture, responsibilities, and dependencies consult `docs/knowledge-base/` first.
Always start with the relevant `index.md` file for the module you're working on.

Key starting points:
- `docs/knowledge-base/overview.md` — project purpose and tech stack
- `docs/knowledge-base/architecture/dependencies.md` — who depends on what (Mermaid graph)
- `docs/knowledge-base/architecture/data-flows.md` — sequence diagrams for send mail + bounce detection
- `docs/knowledge-base/modules/<module>/responsibility.md` — what each module owns
- `docs/knowledge-base/modules/<module>/gotchas.md` — pitfalls and edge cases
- `docs/knowledge-base/cross-cutting/tags.md` — registry of @tag:xxx used throughout the codebase
