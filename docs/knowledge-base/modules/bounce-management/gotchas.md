# bounce-management: Gotchas

## Ticker Fires on Startup AND Every 15 Minutes

The `main.go` calls `crawler.Run(ctx)` immediately on startup, then sets up a `time.NewTicker(15 * time.Minute)`. This means:
- First run: immediately on service start
- Subsequent runs: every 15 minutes

If the initial run is slow (many unread messages), it may overlap with the first ticker event. The code does not prevent concurrent runs — this is acceptable since MS Graph's `MarkAsRead` makes the operation idempotent.

## Per-Message Error Isolation

The `crawler.Run()` loop processes each NDR message independently:
- If `process()` fails for one message, the error is logged and the loop continues
- If `MarkAsRead()` fails for one message, it's logged and the loop continues

This prevents a single corrupt NDR from blocking the entire batch.

## Trace ID Regex Is Simple

The regex `X-Dispatch-TraceId:\s*([0-9a-f-]{36})` matches standard UUID format. If the NDR body doesn't contain this header, `OriginalTraceID` is empty. The bounce record is still published — it just won't correlate to a specific mail.

## BouncedRecipient Is Never Populated

The `BounceRecord.BouncedRecipient` field is always empty string. The current implementation doesn't extract the bounced recipient address from the NDR. This is a known limitation — the field exists in the schema but isn't populated.

## No Unread Message Limit

There's no pagination or limit on `GetUnreadMessages`. If a mailbox has thousands of unread NDRs, the crawler will attempt to process all of them in a single run. Each message requires a PATCH to mark as read, which could be a lot of MS Graph calls.

## Uses msgraph.BounceService (Not the Main Client)

The bounce crawler uses `msgraph.BounceService`, which wraps `msgraph.Client` with NDR-specific methods (`GetUnreadMessages`, `MarkAsRead`). This is separate from `msgraph.Service` which handles email sending. Both share the same underlying `Client` (circuit breaker, retry, token cache).
