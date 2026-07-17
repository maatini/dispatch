# msgraph: Interfaces

## Client Construction

```go
func NewClient(tenantID, clientID, clientSecret, proxyURL, mockToken string) *Client
```

- If `mockToken != ""`, OAuth2 is skipped; token is used directly
- If `proxyURL != ""`, all requests are routed through the proxy with TLS verification disabled
- Circuit breaker: 5 consecutive failures → open, 30s timeout

## Service.SendEmail

```go
func (s *Service) SendEmail(ctx context.Context, req domain.MailRequestDO) error
```

**Behavior:**
- Rate limiter check per sender (`r.Wait(ctx, req.Sender)`)
- Total attachment size < 3 MB → `sendInline()`: single `POST /users/{sender}/sendMail`
- Total attachment size ≥ 3 MB → `sendViaUploadSession()`: create draft → upload attachments → send draft
- Returns `GraphTransientError` or `GraphPermanentError` on failure

## BounceService

```go
type BounceService struct { ... }

func NewBounceService(client *Client) *BounceService

func (s *BounceService) GetUnreadMessages(ctx context.Context, mailbox string) ([]bounce.NDRMessage, error)
// GET /users/{mailbox}/messages?$filter=isRead+eq+false&$select=id,subject,body

func (s *BounceService) MarkAsRead(ctx context.Context, mailbox, messageID string) error
// PATCH /users/{mailbox}/messages/{id}  body: {"isRead":true}
```

## Error Types

```go
// GraphTransientError: 429 / 5xx / IO — worker must NOT ack, JetStream redelivers
type GraphTransientError struct {
    StatusCode int
    RetryAfter time.Duration // from Retry-After header on 429
    Cause      error
}

// GraphPermanentError: 4xx (≠429) — worker must ack and write FAILED to audit
type GraphPermanentError struct {
    StatusCode int
    Body       string
}
```

## RateLimiter

```go
func NewRateLimiter(skipSleep bool) *RateLimiter
func (r *RateLimiter) Wait(ctx context.Context, sender string) error
```

- Per-sender token bucket: 1 req/s, burst 10
- If `skipSleep` is true, `Wait()` returns immediately (dev mode)
- Creates buckets lazily on first use per sender

## Configuration (from config.Config)

| Field | Env Var | Default | Used By |
|---|---|---|---|
| `MSGraphTenantID` | `MS_GRAPH_TENANT_ID` | required | Client → token cache |
| `MSGraphClientID` | `MS_GRAPH_CLIENT_ID` | required | Client → token cache |
| `MSGraphClientSecret` | `MS_GRAPH_CLIENT_SECRET` | required | Client → token cache |
| `GraphProxyURL` | `MS_GRAPH_PROXY_URL` | empty | Client → transport proxy |
| `GraphMockToken` | `MS_GRAPH_MOCK_TOKEN` | empty | Client → skip OAuth2 |
| `GraphRateLimiterSkip` | `DISPATCH_GRAPH_RATE_LIMITER_SKIP_SLEEP` | false | RateLimiter |
