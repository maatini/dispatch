# bounce-management: Responsibilities

## What It Owns

The bounce management service is the **NDR processor** — it periodically polls a dedicated MS Graph bounce mailbox for non-delivery reports, extracts the original trace ID from NDR bodies, and publishes bounce records to NATS for consumption by the admin API.

## Entry Points

| Entry Point | Type | Purpose |
|---|---|---|
| Ticker (15 min) | Timer | Periodic `crawler.Run(ctx)` invocation |
| Immediate startup run | One-shot | First `crawler.Run(ctx)` on service start |

## Processing Pipeline (per tick)

| # | Step | File | What Happens | Failure |
|---|---|---|---|---|
| 1 | Fetch unread NDRs | `crawler.go:56` | `graphClient.GetUnreadMessages(mailbox)` → MS Graph | Error returned, tick fails (logged, next tick retries) |
| 2 | Per-message: extract trace ID | `crawler.go:60` | Regex: `X-Dispatch-TraceId:\s*([0-9a-f-]{36})` on NDR body | Empty string (logged, message still processed) |
| 3 | Per-message: publish bounce | `crawler.go:61-68` | Marshal `BounceRecord` → publish to `DISPATCH_BOUNCES` | Logged, loop continues (doesn't block other messages) |
| 4 | Per-message: mark as read | `crawler.go:69` | `graphClient.MarkAsRead(mailbox, msgID)` → MS Graph PATCH | Logged, loop continues |

## Invariants

| Invariant | Enforcement |
|---|---|
| One bad message doesn't block others | Per-message `process()` errors are logged; loop continues to next message |
| Bounce records include the original trace ID | Regex extraction from NDR body |
| Processed messages are marked as read | `MarkAsRead` called after successful processing |
| Both immediate and periodic execution | Ticker fires on startup and every 15 minutes |

## What It Does NOT Own

- Email delivery — that's mail-worker
- MS Graph email sending — that's msgraph.Service
- Bounce record consumption/querying — that's mail-admin
- Mailbox configuration — that's deployment config (`MS_GRAPH_BOUNCE_MAILBOX` env var)
