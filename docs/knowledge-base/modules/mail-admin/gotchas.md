# mail-admin: Gotchas

## /health Must Be Outside Auth Middleware

The `AuthMiddleware` wraps the GraphQL handler, but `/health` must be registered on a separate route outside the middleware. If `/health` goes through auth middleware, Kubernetes liveness probes will fail without a valid JWT.

Implementation: `main.go` registers `/health` directly on the chi router, then mounts the auth-wrapped GraphQL handler at `/graphql`.

## Stream Reads Use Temporary Subscriptions

All stream queries (`mails`, `bounces`, `deadLetters`) create temporary subscriptions via `js.SubscribeSync("", BindStream(...), DeliverAll())`. These are unsubscribed immediately via `defer`. This means:
- **Performance**: Reads all messages from the stream on each query — fine for moderate volumes, not suitable for large historical data
- **No consumer tracking**: No durable consumer offsets — each query reads everything
- **Termination**: `readStream` waits up to 5s per `NextMsg`, counts every consumed message (even ones that fail to unmarshal) against `StreamInfo.State.Msgs`, and honors request-context cancellation. Corrupt records are skipped, not fatal.

## ReprocessDeadLetter Restores Headers From the Payload

The `reprocessDeadLetter` mutation parses the payload as `MailRequestDO` and republishes it with `traceId`/`appTag` headers (mirroring the gateway publisher), so the worker's dedup keeps working for reprocessed messages. Rules:
- Payload fails to parse → error `invalid dead letter payload`, nothing is published
- Payload has empty `traceId` → a fresh UUID is generated for the header
- Corrupt payloads that once caused the dead letter therefore cannot be blindly requeued

## JWT Auth: No Custom Claims Validation

The `AuthMiddleware` only validates that the token:
1. Is signed with HMAC (rejects RSA/ECDSA)
2. Can be parsed with the secret

It does NOT check `exp` explicitly — the `jwt.Parse()` call with default parser options does this automatically via the `Valid()` method (which checks `exp`, `nbf`, `iat`).

## Sender CRUD Invalidate Cache

`sender.Store.Put()` and `Delete()` explicitly call `delete(s.cache, appTag)`. This ensures the gateway's next lookup sees the updated sender config. The gateway has a 10-min cache TTL, so there may be up to 10 minutes of stale data after an update — this is acceptable for admin operations.

## No PII Masking in Admin Responses

Unlike the gateway and worker (which mask emails in logs), the admin GraphQL responses return raw email addresses to authenticated admin clients. PII masking only applies to `internal/loggy` calls, not to API responses.
