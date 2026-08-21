# bounce-management

**Source:** `cmd/bouncemanagement/`, `internal/bounce/`  
**Role:** Scheduled NDR crawler — poll bounce mailbox → extract trace ID → publish `DISPATCH_BOUNCES`.

## Behavior

- Runs once on startup, then every 15 minutes.
- Serves `GET /metrics` and `/health` on `PORT` (default 8080).
- Uses `msgraph.BounceService` (not send `Service`); same underlying Graph `Client`.
- Per-message isolation: process failure → leave unread for retry; never block batch.
- Trace header regex: `X-Dispatch-TraceId` UUID (set on outbound Graph messages).

## Gotchas

- Single Graph GET, no `@odata.nextLink` — Graph default page (~10). A large unread backlog drains one page per 15 min tick, not as a burst of PATCHes (#21).
- Concurrent run overlap possible if first crawl is slow; MarkAsRead keeps it idempotent enough.
- Missing trace ID still publishes bounce (uncorrelated).
