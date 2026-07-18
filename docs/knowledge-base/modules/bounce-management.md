# bounce-management

**Source:** `cmd/bouncemanagement/`, `internal/bounce/`  
**Role:** Scheduled NDR crawler — poll bounce mailbox → extract trace ID → publish `DISPATCH_BOUNCES`.

## Behavior

- Runs once on startup, then every 15 minutes.
- Uses `msgraph.BounceService` (not send `Service`); same underlying Graph `Client`.
- Per-message isolation: process failure → leave unread for retry; never block batch.
- Trace header regex: `X-Dispatch-TraceId` UUID (set on outbound Graph messages).

## Gotchas

- No unread-message limit/pagination — large backlog = many Graph PATCHes per run.
- Concurrent run overlap possible if first crawl is slow; MarkAsRead keeps it idempotent enough.
- Missing trace ID still publishes bounce (uncorrelated).
