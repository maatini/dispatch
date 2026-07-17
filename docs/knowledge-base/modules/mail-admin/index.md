# mail-admin

JWT-authenticated GraphQL API for operational management: sender CRUD, audit log queries, bounce/dead-letter inspection, and dead-letter reprocessing.

Combined from: `cmd/mail-admin/` + `internal/admin/`

## Files

- **[responsibility.md](responsibility.md)** — What this module owns: sender management, stream queries, reprocessing, auth
- **[dependencies.md](dependencies.md)** — Inbound (GraphQL HTTP) and outbound (NATS KV, streams, JWT)
- **[interfaces.md](interfaces.md)** — GraphQL schema (queries + mutations), JWT auth contract
- **[gotchas.md](gotchas.md)** — JWT HMAC-SHA256, temporary subscriptions for stream reads, auth middleware scope
