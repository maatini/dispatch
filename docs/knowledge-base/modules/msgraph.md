# msgraph

**Source:** `internal/msgraph/`  
**Role:** Graph API client — OAuth token cache, circuit breaker, rate limit, send (inline + upload session), bounce mailbox helpers.

## Layers

| Type | Responsibility |
|------|----------------|
| `Client` | HTTP + gobreaker + retry; classifies 4xx vs 429/5xx |
| `tokenCache` | OAuth2 client-credentials; refresh 60s before expiry; 15s fetch timeout |
| `RateLimiter` | Per-sender token bucket (1 rps, burst 10) |
| `Service` | `SendEmail` — inline if total attach &lt; 3MB else draft + upload session |
| `BounceService` | Unread NDRs + MarkAsRead |

## Gotchas

- **Permanent errors do not trip the breaker** (`IsSuccessful` treats `GraphPermanentError` as success).
- **Open breaker → transient** so worker redelivers, does not FAILED-ack.
- Token **4xx (≠429) → permanent** (bad credentials must not infinite-redeliver).
- `MS_GRAPH_PROXY_URL` → InsecureSkipVerify (dev only). `MS_GRAPH_MOCK_TOKEN` skips OAuth and makes Graph env vars optional.
- Upload chunks: 429/5xx transient; other 4xx permanent (e.g. expired upload URL).
- Draft cleanup on error uses `context.Background()` (best-effort).
