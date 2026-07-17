# services: Responsibilities

## What It Owns

The services module provides **domain services backed by NATS KV** — they encapsulate business logic that operates on persistent state without being tied to any specific service binary (gateway, worker, admin).

## Services

### `internal/quota` (quota.Checker)

**Owns:** Rolling 24-hour recipient quota enforcement with optimistic concurrency (CAS).

**Entry points:**
- `Check(appTag string, limit, requested int) error` — verify and record usage
- `CurrentUsage(appTag string) (int, error)` — current rolling 24h count (for rate limit headers)

**Algorithm:**
1. Read current state from NATS KV keyed by `appTag`
2. Filter out entries older than 24 hours
3. Sum remaining entries + requested, compare to limit
4. If exceeded → return `QuotaError`
5. If ok → write new state with CAS (`Create` if new, `Update` with revision)
6. If CAS conflict → retry up to `maxCASRetries` (10)
7. If all retries exhausted → return `QuotaStateError` (fail-closed)
8. If KV error that is NOT a `nats.JetStreamError` → `QuotaStateError` (network/connection = fail-closed)

**Data structure (per appTag):**
```json
{"entries": [{"ts": 1712345678, "count": 5}, ...]}
```

### `internal/sender` (sender.Store)

**Owns:** Sender configuration CRUD with in-memory TTL cache backed by NATS KV.

**Entry points:**
- `Get(appTag string) (Sender, error)` — lookup with cache; returns `ValidationError` if unknown
- `Put(sender Sender) error` — write to KV, invalidate cache
- `Delete(appTag string) error` — delete from KV, invalidate cache
- `List() ([]Sender, error)` — list all senders (reads all keys from KV)
- `InvalidateCache(appTag string)` — explicit cache invalidation

**Cache behavior:**
- TTL: 10 minutes (configurable via `cacheTTL` parameter)
- Invalidation: automatic on `Put`/`Delete`/`InvalidateCache`
- Read path: check cache → miss → KV read → populate cache

### `internal/spam` (spam.Checker)

**Owns:** Duplicate message detection using SHA-256 hashes in a NATS KV TTL bucket.

**Entry points:**
- `Check(hash string) error` — returns `ValidationError` if hash exists (duplicate), otherwise records hash

**Behavior:**
1. Try to get the hash from the KV bucket
2. If found → return `ValidationError{Code: ErrSpamDetected}` (duplicate)
3. If not found (`ErrKeyNotFound`) → put hash in bucket → return nil (not duplicate)
4. NATS handles TTL expiry automatically — old hashes disappear

## Invariants

| Invariant | Enforcement |
|---|---|
| Quota is fail-closed | Any non-CAS KV error → `QuotaStateError` |
| CAS distinguishes real conflicts from network errors | `errors.As(err, &nats.JetStreamError)` check |
| Sender cache is invalidated on write | `delete(s.cache, appTag)` in `Put`/`Delete` |
| Spam check is atomic within the TTL window | NATS KV provides the atomicity; TTL bucket handles expiry |
| Limit ≤ 0 means unlimited quota | `Check()` returns nil immediately |
| Unknown appTag returns a typed error | `ValidationError{Code: ErrUnknownAppTag}` |

## What It Does NOT Own

- MS Graph communication — that's `internal/msgraph`
- Email delivery — that's `internal/worker`
- Request validation — that's `internal/gateway`
- NATS resource provisioning — that's `internal/natsutil`
