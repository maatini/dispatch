# mail-gateway

HTTP REST entrypoint for mail send requests. Runs a 7-stage validation pipeline, then publishes to NATS JetStream.

Combined from: `cmd/mail-gateway/` + `internal/gateway/`

## Files

- **[responsibility.md](responsibility.md)** — What this module owns, pipeline stages, invariants, entry points
- **[dependencies.md](dependencies.md)** — Inbound (HTTP clients) and outbound (NATS KV, Object Store, JetStream) deps
- **[interfaces.md](interfaces.md)** — HTTP endpoint contract, MailRequest schema, error response format
- **[gotchas.md](gotchas.md)** — Pipeline ordering matters, fail-closed semantics, streaming attachment upload
