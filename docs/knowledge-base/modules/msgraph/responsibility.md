# msgraph: Responsibilities

## What It Owns

The msgraph module is the **MS Graph API abstraction layer** — it encapsulates all HTTP communication with Microsoft Graph, including authentication, resilience patterns, and domain-specific operations (email sending, bounce mailbox polling).

It is a **leaf module** — it depends only on the Go standard library, `internal/loggy`, `internal/domain` (for `MailRequestDO`), and `internal/bounce` (for `NDRMessage`). It has no knowledge of NATS, quotas, senders, or any other internal services.

## Components

| Component | File | Purpose |
|---|---|---|
| `Client` | `client.go` | HTTP client with circuit breaker, token cache, OAuth2, retry with backoff |
| `Service` | `service.go` | Email sending: inline (≤3 MB) or upload session (>3 MB) |
| `BounceService` | `bounce.go` | Bounce mailbox polling: get unread messages, mark as read |
| `RateLimiter` | `ratelimiter.go` | Per-sender token bucket rate limiter (1 req/s, burst 10) |
| `tokenCache` | `token.go` | OAuth2 client credentials token cache with 60s expiry buffer |
| `errors` | `errors.go` | `GraphTransientError` (429/5xx/IO) vs `GraphPermanentError` (4xx≠429) |

## Resilience Features

| Feature | Implementation | Configuration |
|---|---|---|
| Circuit breaker | `sony/gobreaker` | Opens after 5 consecutive failures, 30s timeout, half-open with 1 test request |
| Retry | Up to 3 attempts | Retry-After on 429 (max 30s), 2s fallback for 5xx |
| Token cache | In-memory with 60s buffer | OAuth2 client credentials flow |
| Rate limiter | Per-sender `x/time/rate` token bucket | 1 req/s, burst 10; can be disabled via `DISPATCH_GRAPH_RATE_LIMITER_SKIP_SLEEP` |
| Dev proxy | Route through `MS_GRAPH_PROXY_URL` | TLS verification disabled (dev only) |
| Mock token | Skip OAuth2 via `MS_GRAPH_MOCK_TOKEN` | Makes Graph credentials optional |

## Email Send Strategies

| Strategy | Threshold | Flow |
|---|---|---|
| Inline (`sendInline`) | Total attachment size < 3 MB | `POST /users/{sender}/sendMail` with base64-embedded attachments |
| Upload session (`sendViaUploadSession`) | Total attachment size ≥ 3 MB | Create draft → small attachments via `POST .../attachments` → large attachments via upload session → `POST .../send` |

## Invariants

| Invariant | Enforcement |
|---|---|
| Circuit breaker only counts transient errors | `GraphPermanentError` is marked as "successful" in breaker config |
| Circuit breaker failure blocks all requests | `breaker.Execute()` wraps every request via `do()` |
| Token is refreshed before expiry | `tokenCache.get()` checks `expiresAt - 60s` buffer |
| Chunked upload uses 1.25 MB chunks | `chunkSize = 4 * 327_680` bytes |
| Draft is deleted on any attachment upload failure | `cleanup()` function in `sendViaUploadSession` |
| Dev proxy mode disables TLS verification | `InsecureSkipVerify: true` only when `proxyURL != ""` |

## What It Does NOT Own

- NATS communication — that's `natsutil`
- Quota, spam, sender config — those are in `internal/quota`, `internal/spam`, `internal/sender`
- MailRequest → Graph payload transformation — that's `buildGraphEmail()` (internal to this package)
- Worker dedup or audit — that's `internal/worker`
