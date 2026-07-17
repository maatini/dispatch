# mail-admin: Gotchas

## /health Must Be Outside Auth Middleware

The `AuthMiddleware` wraps the GraphQL handler, but `/health` must be registered on a separate route outside the middleware. If `/health` goes through auth middleware, Kubernetes liveness probes will fail without a valid JWT.

Implementation: `main.go` registers `/health` directly on the chi router, then mounts the auth-wrapped GraphQL handler at `/graphql`.

## Stream Reads Use Temporary Subscriptions

All stream queries (`mails`, `bounces`, `deadLetters`) create temporary subscriptions via `js.SubscribeSync("", BindStream(...), DeliverAll())`. These are unsubscribed immediately via `defer`. This means:
- **Performance**: Reads all messages from the stream on each query — fine for moderate volumes, not suitable for large historical data
- **No consumer tracking**: No durable consumer offsets — each query reads everything

## ReprocessDeadLetter Publishes Raw Payload

The `reprocessDeadLetter` mutation takes a raw JSON string and publishes it directly to `DISPATCH_MAILS`. There is no validation of the payload. This is intentional — the original gateway already validated it. If the payload was malformed, it'll end up back in the dead letter stream.

## JWT Auth: No Custom Claims Validation

The `AuthMiddleware` only validates that the token:
1. Is signed with HMAC (rejects RSA/ECDSA)
2. Can be parsed with the secret

It does NOT check `exp` explicitly — the `jwt.Parse()` call with default parser options does this automatically via the `Valid()` method (which checks `exp`, `nbf`, `iat`).

## Sender CRUD Invalidate Cache

`sender.Store.Put()` and `Delete()` explicitly call `delete(s.cache, appTag)`. This ensures the gateway's next lookup sees the updated sender config. The gateway has a 10-min cache TTL, so there may be up to 10 minutes of stale data after an update — this is acceptable for admin operations.

## No PII Masking in Admin Responses

Unlike the gateway and worker (which mask emails in logs), the admin GraphQL responses return raw email addresses to authenticated admin clients. PII masking only applies to `internal/loggy` calls, not to API responses.
