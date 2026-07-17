# mail-admin: Responsibilities

## What It Owns

The mail-admin is the **operational management interface** — it provides a GraphQL API for managing sender configurations, querying audit/bounce/dead-letter streams, and reprocessing dead letters. All operations require JWT authentication.

## Entry Points

| Entry Point | Type | Purpose |
|---|---|---|
| `POST /graphql` | GraphQL endpoint | All queries and mutations (JWT required) |
| `GET /health` | HTTP endpoint | Health check |

## GraphQL Operations

### Queries

| Query | Args | Returns | Implementation |
|---|---|---|---|
| `senders` | `filter: { appTag }` (optional) | `[Sender]` | `sender.Store.List()` with optional filter |
| `mails` | `filter: { appTag, status, traceId }`, `page`, `size` | Paged `[AuditRecord]` | Reads `DISPATCH_AUDIT` stream via temporary subscription |
| `bounces` | `page`, `size` | Paged `[BounceRecord]` | Reads `DISPATCH_BOUNCES` stream via temporary subscription |
| `deadLetters` | `page`, `size` | Paged `[DeadLetter]` | Reads `DISPATCH_DEAD_LETTERS` stream via temporary subscription |

### Mutations

| Mutation | Input | Returns | Implementation |
|---|---|---|---|
| `createSender` | `{ appTag, email, test, dailyQuota, allowedDomains }` | `Sender` | `sender.Store.Put()` |
| `updateSender` | `appTag` + same fields as create | `Sender` | `sender.Store.Put()` (overwrites) |
| `deleteSender` | `appTag` | `Boolean` | `sender.Store.Delete()` |
| `reprocessDeadLetter` | `payload` | `Boolean` | `js.Publish(natsutil.SubjectMails, payload)` |

## Invariants

| Invariant | Enforcement |
|---|---|
| All operations require JWT auth | `AuthMiddleware` wraps the GraphQL handler; `/health` is registered outside the middleware |
| Token uses HMAC-SHA256 with `DISPATCH_ADMIN_AUTH_SECRET` | `jwt.Parse()` checks signing method |
| Only `exp` claim is required | Standard JWT validation (no custom claims) |
| Stream reads use temporary subscriptions | `SubscribeSync("", BindStream(...), DeliverAll())` + `Unsubscribe()` in defer |
| Sender CRUD invalidates cache | `sender.Store.Put()` and `Delete()` call `delete(s.cache, appTag)` |

## What It Does NOT Own

- Token generation — that's `tools/gen-admin-token/` (CLI tool)
- Email delivery — that's mail-worker
- Request validation / quota — that's mail-gateway
- Bounce detection — that's bouncemanagement
