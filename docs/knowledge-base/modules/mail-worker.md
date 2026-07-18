# mail-worker

**Source:** `cmd/mail-worker/`, `internal/worker/`  
**Role:** Pull consumer → dedup → MaxDeliver gate → MS Graph send → audit / DLQ.

## Flow

1. InProgress heartbeat (`AckWait/3`, min 10s)
2. JSON parse fail → DLQ + ACK
3. **Dedup Get** (`delivered` KV) — found → ACK skip; KV error → no ACK
4. **MaxDeliver gate** — `NumDelivered >= maxDeliver` → DLQ + FAILED + Term (no Graph)
5. Attachment Fetch → fail → no ACK
6. Test flag → TEST_SUCCESS + Put + ACK + cleanup
7. Graph send: transient → no ACK; permanent → FAILED + ACK; success → DELIVERED + **Put then ACK** (Put fail → no ACK)

Defaults: AckWait 5m, MaxDeliver 8 (`DISPATCH_WORKER_*`). `ProvisionWorkerConsumer` creates **and** updates the consumer.

## Gotchas

- **Dedup before MaxDeliver** — already-delivered must not become false FAILED after high `NumDelivered`.
- **Get and Put both fail-closed** — double-send worse than redelivery; Put failure skips attachment cleanup.
- **Transient vs permanent Graph errors** — mix-up loses messages or infinite-loops.
- **Cleanup is best-effort** — Object Store TTL (72h) is the safety net.
- Pull consumer: `Fetch(10)` loop, explicit ACK. Unit tests without JetStream metadata skip MaxDeliver gate fail-soft.
