# bounce-management

Scheduled background job (every 15 minutes) that polls a dedicated MS Graph mailbox for NDR (non-delivery report) messages, extracts trace IDs, and publishes bounce records to NATS.

Combined from: `cmd/bouncemanagement/` + `internal/bounce/`

## Files

- **[responsibility.md](responsibility.md)** — What this module owns: NDR crawling, trace ID extraction, bounce publishing
- **[dependencies.md](dependencies.md)** — Inbound (ticker) and outbound (msgraph bounce API, NATS stream)
- **[interfaces.md](interfaces.md)** — BounceRecord schema, NDRMessage contract, graphClient interface
- **[gotchas.md](gotchas.md)** — 15-min ticker, regex extraction, per-message error isolation
