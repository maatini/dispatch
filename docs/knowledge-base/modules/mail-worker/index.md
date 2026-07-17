# mail-worker

NATS JetStream pull consumer that deserializes mail jobs, fetches attachments, delivers via MS Graph, and writes audit records.

Combined from: `cmd/mail-worker/` + `internal/worker/`

## Files

- **[responsibility.md](responsibility.md)** — What this module owns: message processing, dedup, delivery orchestration, audit
- **[dependencies.md](dependencies.md)** — Inbound (NATS consumer) and outbound (msgraph, NATS KV, streams)
- **[interfaces.md](interfaces.md)** — MailRequestDO schema, NATS subject contracts, consumer-side interfaces
- **[gotchas.md](gotchas.md)** — Transient vs permanent error distinction, attachment cleanup, dedup ordering
