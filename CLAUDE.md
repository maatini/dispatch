# CLAUDE.md – dispatch (Multi-Tenant Email Delivery)

**Ziel (WHY):** Zuverlässige, skalierbare, mandantenfähige E-Mail-Zustellung über MS Graph mit strikten Quota-, Spam- und Deduplikationsregeln. Zero Double-Delivery. Fail-Closed Philosophie.

**Wichtige Commands (immer `devbox run …`):**
```bash
devbox run build && devbox run test && devbox run lint
devbox run up                 # NATS (+ optional Graph Proxy)
devbox run down
devbox run test-integration   # needs NATS; //go:build integration
```

## 0. Agent / Harness notes

- Primary rules for **Claude Code and Grok**: this file. Deeper dirs inherit it.
- If lean-ctx / RTK tools are unavailable: use native file + shell tools; **never block** on them.
- Prefer `devbox run <script>` over bare `go test` / `golangci-lint` outside devbox.
- On rule conflict: **this file's invariants beat generic style preferences** (simplicity over “elegant” rewrites).
- Continuity: before non-trivial work, skim **Hot decisions** + last **5** entries in `docs/ai-changes.md` — do **not** re-read the whole log.
- Open product work lives in `improvements.md` only — do not invent parallel backlogs.
- Language: **code/comments EN**; conversation with the user may be DE.

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
- Env prefix: `DISPATCH_*` — **never invent** new vars without updating `internal/config` + README

## 3. Project Structure (High-Level)

```
cmd/
  mail-gateway/     # REST entrypoint (POST /dispatch/api/v1/mail/send)
  mail-worker/      # NATS Pull-Consumer → MS Graph delivery
  mail-admin/       # GraphQL tenant/sender/audit management
  bouncemanagement/ # Scheduled NDR crawler
internal/           # domain models, services, clients (incl. loggy, natsutil, httpsrv, testkit)
docs/
  ai-changes.md     # AI change log (+ Hot decisions at top)
  knowledge-base/   # modules, ADRs, shared patterns
ARCHITECTURE.md     # detailed pipeline (German) — canonical diagrams
README.md           # user-facing overview + API
improvements.md     # open backlog only
```

## 4. Core Domain Invariants (MUST)

- `appTag` = unique Tenant identifier
- **Quota**: rolling 24h window (TO+CC+BCC), optimistic CAS via NATS KV, **fail-closed** (any error → HTTP 503)
- **Spam cache**: SHA-256 fingerprint (`spam.Hash`), bucket TTL 60s
- **Delivered dedup**: 7-day TTL in NATS KV; Get **and** Put fail-closed (Put fail → no ACK)
- **Gateway AuthN**: Bearer `DISPATCH_GATEWAY_AUTH_TOKEN` on `/mail/send` (health open; disable only via `DISPATCH_GATEWAY_AUTH_DISABLED` for local)
- **Never** bypass quota check or deduplication

## 5. Coding Conventions & Go Idioms

Short list; details + examples: `docs/knowledge-base/cross-cutting/shared-patterns.md`.

- Errors: wrap with `fmt.Errorf("...: %w", err)`; inspect with `errors.Is` / `errors.As`
- Interfaces: define at point of **use** (consumer), never at producer
- Context: first param for I/O; never store in structs; no `context.Background()` deep in the stack
- Logging: only `loggy.GetLogger("ComponentName")` + semantic methods; PII via `loggy.MaskEmail`
- No `init()`, no global mutable state

## 6. Error Handling & Resilience

- Quota / NATS publish / Attachment upload errors → HTTP 503 (fail-closed)
- MS Graph 429/5xx → **no ACK** (redelivery; Retry-After ≤ 30s); worker InProgress heartbeat (AckWait 5m default)
- MaxDeliver exhausted (default 8) → DLQ + FAILED + Term, no Graph; **Dedup Get before MaxDeliver gate**
- MS Graph 4xx → ACK + FAILED in `DISPATCH_AUDIT`
- Malformed JSON → ACK + `DISPATCH_DEAD_LETTERS`

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
2. Append a short entry to `docs/ai-changes.md` (strict format – max. 5 lines body). Update **Hot decisions** if a MUST/ADR changed.
3. For complex tasks (> 3 steps or any uncertainty): **propose a plan first** and wait for confirmation.

**At the end of longer sessions:** suggest which insights belong in `CLAUDE.md`, `ARCHITECTURE.md`, or `docs/knowledge-base/` (do not dump the full log).

Mark important design decisions with `**WICHTIG**` or `**DESIGN-DECISION**`.

## 9. Task router (first read)

| Task | Read first |
|------|------------|
| Send / HTTP / Quota / Spam / AuthN | `docs/knowledge-base/modules/mail-gateway.md` + `ARCHITECTURE.md` |
| ACK / Dedup / MaxDeliver / DLQ | `docs/knowledge-base/modules/mail-worker.md` |
| Graph / 429 / CB / uploads | `docs/knowledge-base/modules/msgraph.md` |
| Admin GraphQL / JWT | `docs/knowledge-base/modules/mail-admin.md` |
| Bounce / NDR | `docs/knowledge-base/modules/bounce-management.md` |
| ADRs (NATS-only, fail-closed, …) | `docs/knowledge-base/decisions.md` |
| Logging / interfaces / errors style | `docs/knowledge-base/cross-cutting/shared-patterns.md` |
| Open backlog | `improvements.md` |
| Recent rationale | `docs/ai-changes.md` (Hot decisions + last 5) |

Pipeline diagrams & NATS ownership: **only** root `ARCHITECTURE.md` (not duplicated in KB).

Full KB map: `docs/knowledge-base/index.md`.

**Last updated:** 2026-07-18 | Version: 2.5 (agent harness notes + task router)
