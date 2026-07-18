# Architectural Decisions

Key decisions and their rationale. Sourced from `ARCHITECTURE.md`, `CLAUDE.md`, and `docs/ai-changes.md`.

## NATS-Only State Backend

**Decision:** Use NATS JetStream (KV + Streams + Object Store) as the sole state backend. No PostgreSQL, Redis, SQLite, or any external database.

**Rationale:**
- Eliminates operational complexity of managing multiple state stores
- NATS JetStream provides all needed primitives: durable streams (work queues), KV buckets (with TTL), and object store (binary blobs)
- The system's state is naturally short-lived (quota windows, spam windows, attachment blobs) — no need for long-term relational data
- Simplifies deployment: only NATS + the Go binaries, no database to manage

**Enforcement:** `CLAUDE.md` explicitly forbids introducing new state backends.

---

## Fail-Closed for Quota and NATS Errors

**Decision:** Any error during quota check, NATS publish, or attachment upload results in HTTP 503 (Service Unavailable). Never bypass these checks.

**Rationale:**
- Multi-tenant system — one tenant's over-sending could degrade service for others
- Quota is a hard contractual boundary; bypassing it would violate tenant agreements
- "Better to reject than to over-deliver or double-deliver"

**Enforcement:** `quota.Checker.Check()` returns `QuotaStateError` on KV errors; gateway handler maps this to 503. No code path exists that bypasses quota.

---

## Deduplication via NATS KV `delivered`

**Decision:** Use a NATS KV bucket (`delivered`, 7-day TTL) keyed by `traceID` to guarantee zero double-delivery.

**Rationale:**
- Worker may crash after MS Graph confirms delivery but before ACKing the NATS message
- On redelivery, the worker checks `delivered` KV — if traceID exists, it ACKs immediately without re-sending
- 7-day TTL is safely longer than any realistic JetStream redelivery window

**Enforcement:** The dedup **Get** runs before any external call. After a successful send, the dedup **Put** must succeed before ACK (fail-closed: Put error → no ACK, no attachment cleanup). Double-send is worse than redelivery.

---

## Consumer-Side Interface Definition

**Decision:** Go interfaces are defined at the point of use (consumer package), not at the point of definition (producer package).

**Rationale:**
- Follows Go best practice: "interfaces should be defined where they are used"
- Keeps packages decoupled — the consumer declares only the methods it actually needs
- Makes testing trivial: the consumer's interface is already the mock surface
- Examples: `gateway.Handler` defines `senderLookup`, `quotaChecker`, `spamChecker` interfaces; `worker.Processor` defines `emailSender`, `deliveredStore`, `attachmentFetcher` interfaces

---

## Exclusive loggy Usage

**Decision:** All logging must go through `internal/loggy` — never call `slog.*`, `log.*`, or `fmt.Print*` directly in production code.

**Rationale:**
- Enforces structured JSON logging with semantic categories (`"type"` field)
- Provides API latency tracking (`RecordApiStart` / `ExternalApiSuccess` / `ExternalApiFailure`)
- Ensures PII masking is always applied (email addresses masked via `MaskEmail`)
- Each component gets its own logger via `loggy.GetLogger("ComponentName")` for clear log attribution

**Enforcement:** `CLAUDE.md` explicitly forbids direct `slog.*`/`fmt.Println` calls; `go vet` would catch unused `slog` imports.

---

## 7-Stage Gateway Pipeline Ordering

**Decision:** The gateway pipeline stages are executed in this fixed order: JSON decode → struct validation → sender lookup → domain whitelist → quota check → spam dedup → attachment upload → NATS publish.

**Rationale:**
- Early rejection is cheap: reject invalid requests before hitting NATS KV
- Sender lookup is needed before domain whitelist (need AllowedDomains from sender config)
- Domain whitelist is needed before quota (shouldn't count recipients toward quota if domain is blocked)
- Quota before spam (quota is a hard limit, spam is a soft duplicate check)
- Spam before attachment upload (don't store attachments for duplicate messages)
- Attachment upload before NATS publish (don't publish if Object Store is unavailable)

---

## 4 Independent Binaries (Not Monolith)

**Decision:** Package as 4 separate Go binaries (`mail-gateway`, `mail-worker`, `mail-admin`, `bouncemanagement`) instead of a single monolith.

**Rationale:**
- Independent scaling: gateway can scale with HTTP load; worker scales with email volume
- Different deployment profiles: gateway needs HTTP ingress; worker and bouncemanagement don't
- Failure isolation: a gateway crash doesn't affect in-flight worker processing
- NATS provides all inter-service communication; no shared memory needed

---

## Distroless Container Images

**Decision:** Docker images use `gcr.io/distroless/static-debian12:nonroot` with multi-stage builds.

**Rationale:**
- Minimal attack surface: no shell, no package manager, no utilities
- Non-root user by default
- Smaller images than full Debian/Alpine bases
- Go static binaries are ideal for distroless (no glibc dependency)
