# mail-worker: Gotchas

## Transient vs Permanent Error Distinction Is Critical

The worker MUST distinguish between `GraphTransientError` and `GraphPermanentError` correctly:
- **Transient (no ACK):** 429, 5xx, IO errors → message stays in JetStream for redelivery
- **Permanent (ACK + FAILED):** 4xx (except 429) → message removed, failure recorded

Mixing these up would cause either:
- Lost messages (ACK on transient → never redelivered)
- Infinite loops (no ACK on permanent → infinite redelivery)

The distinction is enforced via `errors.As(err, &transient)` in `processor.processSend()`.

## Dedup Check Comes Before Any External Call

The `delivered` KV check runs before the MS Graph send and before the attachment fetch. If a worker crashes after sending but before ACKing, the redelivered message will find the traceID in `delivered` KV and skip. This guarantees zero double-delivery.

**TraceID source:** the dedup key is `req.TraceID` from the payload, with the `traceId` NATS header only as fallback. If both are empty, the message goes to the dead-letter stream (reason `missing traceId`) and is ACKed — there is no shared `"unknown"` dedup key, so headerless messages can never collide.

**Important:** The `delivered` KV write happens AFTER MS Graph success. If the write fails, it's logged but the ACK still happens — the 7-day TTL means the next delivery attempt for the same traceID would be skipped anyway.

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
