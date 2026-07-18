# mail-worker: Gotchas

## Transient vs Permanent Error Distinction Is Critical

The worker MUST distinguish between `GraphTransientError` and `GraphPermanentError` correctly:
- **Transient (no ACK):** 429, 5xx, IO errors → message stays in JetStream for redelivery
- **Permanent (ACK + FAILED):** 4xx (except 429) → message removed, failure recorded

Mixing these up would cause either:
- Lost messages (ACK on transient → never redelivered)
- Infinite loops (no ACK on permanent → infinite redelivery)

The distinction is enforced via `errors.As(err, &transient)` in `processor.processSend()`.

## AckWait 5m + InProgress — Do Not Rely on Short AckWait Alone

Defaults: `AckWait=5m` (`DISPATCH_WORKER_ACK_WAIT_SECONDS`), `MaxDeliver=8` (`DISPATCH_WORKER_MAX_DELIVER`). Infinite MaxDeliver (`-1`) is rejected by config.

While `Handle` runs, a ticker calls `msg.InProgress()` every `AckWait/3` (min 10s) so Graph retries / attachment work do not redeliver mid-flight. InProgress failures are warn-only (non-fatal).

`ProvisionWorkerConsumer(js, ackWait, maxDeliver)` **updates** an existing consumer — restart after deploy is required for live clusters to pick up new values (create-only would leave 30s/-1 forever).

## MaxDeliver Gate: Dedup Before Gate, Then DLQ + Term

Order is intentional:
1. Dedup Get first — if already delivered, Ack-skip **even** when `NumDelivered` is high (no false FAILED/DLQ after successful Put).
2. Then if `NumDelivered >= maxDeliver`: write dead letter (`max deliver exceeded: N`), FAILED audit, best-effort attachment cleanup, `msg.Term()` (Ack fallback). **No Graph call.**

Unit tests without JetStream reply subjects skip the gate fail-soft (`Metadata()` fails → no count).

## Dedup Is Fail-Closed (Get and Put)

**Get** (`delivered.Get(traceID)`):
- **Key found** → duplicate, ACK and skip (idempotent delivery guarantee)
- **`nats.ErrKeyNotFound`** → key not found, proceed with send
- **Any other error** (KV unreachable, timeout) → **no ACK, no Graph call** — JetStream redelivers

**Put** (after Graph/Test success):
- Put **must succeed before ACK**. Put failure → **no ACK**, no attachment cleanup (redelivery needs objects).
- Tradeoff: double-send is worse than redelivery; redelivery is safe if Get then finds the key from a partial success, or Graph may send again only if Put never landed — prefer redelivery over silent ACK without dedup key.

Both paths are fail-closed: unavailable `delivered` KV delays delivery instead of risking double-send.

## Attachment Fetch Failure → No ACK

If the Object Store is unreachable during attachment fetch, the worker does NOT ack. JetStream redelivers. The attachments remain in the Object Store (they were put there by the gateway). On redelivery, fetch is retried.

## Attachment Cleanup Is Best-Effort

`worker.AttachmentStore.Cleanup()` logs errors but never returns them. The 72h bucket TTL is the safety net for orphaned objects. This is intentional — a failed cleanup should not block the ACK.

## Test Mode Bypasses MS Graph Entirely

When `req.Test` is true (set from `sender.Test`), the worker:
1. Skips MS Graph entirely
2. Writes `TEST_SUCCESS` audit record
3. Still writes to `delivered` KV
4. Still cleans up attachments

This is used for integration testing without real email delivery.

## Consumer Uses Explicit Fetch (Not Push-Based)

The consumer calls `sub.Fetch(10)` in a loop — it's pull-based, not push-based. This gives explicit control over concurrency and backpressure. The fetch timeout is context-based.

## Logger Is Context-Enriched Per Message

Each message handler creates a context-enriched logger: `procLog.With(loggy.Kv("traceId", traceID))`. This ensures every log line for a given message includes the trace ID for correlation.
