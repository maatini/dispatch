# mail-worker: Responsibilities

## What It Owns

The mail worker is the **delivery orchestrator** — it consumes messages from NATS JetStream, handles deduplication, fetches attachments, sends emails via MS Graph, writes audit records, and manages error handling semantics.

## Entry Points

| Entry Point | Type | Purpose |
|---|---|---|
| `Consumer.Run(ctx)` | Long-running loop | Pull messages from `DISPATCH_MAILS` (batch of 10, durable consumer `mail-worker`) |
| `Processor.Handle(ctx, msg)` | Per-message handler | Full pipeline: InProgress → deserialize → dedup → MaxDeliver gate → fetch → send → audit → cleanup |

## Processing Pipeline (per message)

| # | Step | File | What Happens | Failure |
|---|---|---|---|---|
| 0 | InProgress heartbeat | `processor.go` | Ticker `msg.InProgress()` every AckWait/3 (min 10s); defer stop | Warn-only on signal failure |
| 1 | JSON deserialize | `processor.go` | Unmarshal `MailRequestDO` from NATS message data | ACK + write dead letter |
| 2 | Dedup check | `processor.go` | Check `delivered` KV for existing `traceID` (**before** MaxDeliver) | ACK (skip, already delivered) |
| 3 | MaxDeliver gate | `processor.go` | If `NumDelivered >= maxDeliver` and not delivered | DLQ + FAILED audit + `Term` (no Graph) |
| 4 | Attachment fetch | `processor.go` | Download attachment bytes from Object Store | No ACK → JetStream redelivers |
| 5 | Test mode | `processor.go` | If `req.Test == true`, skip MS Graph, write `TEST_SUCCESS` audit | Put fail-closed |
| 6 | MS Graph send | `processor.go` → `msgraph.Service.SendEmail()` | Inline (≤3 MB) or upload session (>3 MB) | See error semantics below |
| 7 | Audit write | `processor.go` | Publish `AuditRecord` to `DISPATCH_AUDIT` | Logged, but does not block |
| 8 | Dedup record | `processor.go` | Write `traceID` to `delivered` KV | **No ACK** if Put fails (fail-closed) |
| 9 | Attachment cleanup | `processor.go` | Delete objects from Object Store | Only after successful Put; best-effort (TTL handles orphans) |
| 10 | ACK | `processor.go` | Acknowledge message to NATS | Only after successful `delivered` Put |

## Error Semantics

| Error Type | Behavior |
|---|---|
| JSON parse error | ACK + write `DeadLetter` to `DISPATCH_DEAD_LETTERS` |
| MaxDeliver exhausted | DLQ (`max deliver exceeded: N`) + FAILED audit + `Term` (fallback Ack); **no Graph** |
| `GraphTransientError` (429/5xx/IO) | **No ACK** → JetStream redelivers; attachments kept in Object Store for next attempt |
| `GraphPermanentError` (4xx≠429) | ACK + write `FAILED` to `DISPATCH_AUDIT` + cleanup attachments |
| Attachment fetch error | No ACK → JetStream redelivers |
| Audit write failure | Logged; does not block Put/ACK |
| `delivered` KV write failure | **No ACK**, no attachment cleanup → JetStream redelivers |

## Invariants

| Invariant | Enforcement |
|---|---|
| Zero double-delivery | Dedup Get before Graph **and** before MaxDeliver gate; Dedup Put must succeed before ACK |
| Transient errors never lose messages | No ACK → JetStream holds and redelivers (until MaxDeliver) |
| Permanent errors are terminal | ACK + FAILED audit → message removed from work queue |
| Poison messages terminate | Finite MaxDeliver (default 8) + app-side DLQ/Term |
| Long work does not redeliver mid-flight | InProgress heartbeat resets AckWait timer |
| Attachment cleanup eventually happens | Explicit cleanup on success/failure; bucket TTL (72h) handles orphans |
| Test mode never calls MS Graph | `req.Test` checked before any MS Graph call |
| PII is masked in all logs | `pii.MaskEmail()` on sender + recipient addresses |

## What It Does NOT Own

- Quota enforcement — that's mail-gateway
- Spam deduplication — that's mail-gateway
- Sender validation — that's mail-gateway
- Bounce processing — that's bouncemanagement
- Sender configuration management — that's mail-admin
