# mail-admin

**Source:** `cmd/mail-admin/`, `internal/admin/`  
**Role:** GraphQL admin API — sender CRUD, audit/bounce/DL queries, dead-letter reprocess.

## Surface

- `POST /graphql` — JWT HMAC auth (`DISPATCH_ADMIN_AUTH_SECRET`); `exp` **required** (`jwt.WithExpirationRequired`)
- `GET /health` — **outside** auth middleware (probes)
- Queries: `senders`, `mails`, `bounces`, `deadLetters` (in-memory page after full stream scan)
- Mutations: `createSender` / `updateSender` / `deleteSender`, `reprocessDeadLetter`

## Gotchas

- Stream reads use temporary `SubscribeSync` + DeliverAll — OK for moderate volume; not for large history (#17 backlog).
- `reprocessDeadLetter` republishes with `traceId`/`appTag` headers so worker dedup works; invalid payload → error, no publish.
- Admin API returns **raw** emails to authenticated clients; PII masking is for logs only (`loggy.MaskEmail`).
- Sender Put/Delete invalidates **local** cache only — other gateway replicas may stale up to 10 min.
- Local tokens: `tools/gen-admin-token` (always sets `exp`).
