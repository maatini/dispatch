# mail-worker: Responsibilities

## What It Owns

The mail worker is the **delivery orchestrator** — it consumes messages from NATS JetStream, handles deduplication, fetches attachments, sends emails via MS Graph, writes audit records, and manages error handling semantics.

## Entry Points

| Entry Point | Type | Purpose |
|---|---|---|
| `Consumer.Run(ctx)` | Long-running loop | Pull messages from `DISPATCH_MAILS` (batch of 10, durable consumer `mail-worker`) |
| `Processor.Handle(ctx, msg)` | Per-message handler | Full processing pipeline: deserialize → dedup → fetch → send → audit → cleanup |

## Processing Pipeline (per message)

| # | Step | File | What Happens | Failure |
|---|---|---|---|---|
| 1 | JSON deserialize | `processor.go:52-59` | Unmarshal `MailRequestDO` from NATS message data | ACK + write dead letter |
| 2 | Dedup check | `processor.go:62-66` | Check `delivered` KV for existing `traceID` | ACK (skip, already delivered) |
| 3 | Attachment fetch | `processor.go:69-75` | Download attachment bytes from Object Store | No ACK → JetStream redelivers |
| 4 | Test mode | `processor.go:77-80` | If `sender.Test == true`, skip MS Graph, write `TEST_SUCCESS` audit | (none — always succeeds) |
| 5 | MS Graph send | `processor.go:81` → `msgraph.Service.SendEmail()` | Inline (≤3 MB) or upload session (>3 MB) | See error semantics below |
| 6 | Audit write | `processor.go:85-87` | Publish `AuditRecord` to `DISPATCH_AUDIT` | Logged, but does not block |
| 7 | Dedup record | `processor.go:88-90` | Write `traceID` to `delivered` KV | Logged, no retry |
| 8 | Attachment cleanup | `processor.go:91-93` | Delete objects from Object Store | Logged, no retry (TTL handles orphans) |
| 9 | ACK | `processor.go:94` | Acknowledge message to NATS | — |

## Error Semantics

| Error Type | Behavior |
|---|---|
| JSON parse error | ACK + write `DeadLetter` to `DISPATCH_DEAD_LETTERS` |
| `GraphTransientError` (429/5xx/IO) | **No ACK** → JetStream redelivers; attachments kept in Object Store for next attempt |
| `GraphPermanentError` (4xx≠429) | ACK + write `FAILED` to `DISPATCH_AUDIT` + cleanup attachments |
| Attachment fetch error | No ACK → JetStream redelivers |
| Audit write failure | Logged; does not block ACK |
| `delivered` KV write failure | Logged; does not block ACK |

## Invariants

| Invariant | Enforcement |
|---|---|
| Zero double-delivery | Dedup check (`delivered` KV) before any MS Graph call |
| Transient errors never lose messages | No ACK → JetStream holds and redelivers |
| Permanent errors are terminal | ACK + FAILED audit → message removed from work queue |
| Attachment cleanup eventually happens | Explicit cleanup on success/failure; bucket TTL (72h) handles orphans |
| Test mode never calls MS Graph | `sender.Test` checked before any MS Graph call |
| PII is masked in all logs | `pii.MaskEmail()` on sender + recipient addresses |

## What It Does NOT Own

- Quota enforcement — that's mail-gateway
- Spam deduplication — that's mail-gateway
- Sender validation — that's mail-gateway
- Bounce processing — that's bouncemanagement
- Sender configuration management — that's mail-admin
