# bounce-management: Interfaces

## BounceRecord (published to `DISPATCH_BOUNCES`)

```go
type BounceRecord struct {
    OriginalTraceID  string    `json:"originalTraceId"`
    BouncedAt        time.Time `json:"bouncedAt"`
    BounceReason     string    `json:"bounceReason"`
    BouncedRecipient string    `json:"bouncedRecipient"` // empty — not extracted from NDR
    ProcessedAt      time.Time `json:"processedAt"`
}
```

## NDRMessage (returned from MS Graph)

```go
type NDRMessage struct {
    ID      string // MS Graph message ID
    Body    string // HTML/plain text body content
    Subject string // Email subject
}
```

## graphClient Interface

```go
type graphClient interface {
    GetUnreadMessages(ctx context.Context, mailbox string) ([]NDRMessage, error)
    MarkAsRead(ctx context.Context, mailbox, messageID string) error
}
```

**MS Graph API calls behind this interface:**
- `GetUnreadMessages`: `GET /users/{mailbox}/messages?$filter=isRead+eq+false&$select=id,subject,body`
- `MarkAsRead`: `PATCH /users/{mailbox}/messages/{id}` with body `{"isRead":true}`

## Trace ID Extraction

Regex used: `X-Dispatch-TraceId:\s*([0-9a-f-]{36})`

The trace ID is extracted from the NDR email body (not headers — NDRs forward the original headers as body text).
If extraction fails, the bounce record is still published with `OriginalTraceID` set to empty string.

## Configuration

| Env Var | Default | Purpose |
|---|---|---|
| `MS_GRAPH_BOUNCE_MAILBOX` | `MS_GRAPH_SENDER_EMAIL` or `noreply@dev.local` | Mailbox to poll for NDRs |
