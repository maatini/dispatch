# services: Gotchas

## Quota CAS: Distinguishing Conflict From Failure

`quota.Checker.attempt()` uses `errors.As(err, &nats.JetStreamError)` to distinguish:
- **CAS conflict** (wrong sequence or key exists) → `JetStreamError` → retry (up to 10 times)
- **Network/connection error** → NOT a `JetStreamError` → `QuotaStateError` → fail-closed (no retry)

This is critical: retrying on a network error when NATS is down would just waste time. Checking `JetStreamError` is the correct discriminator.

## Quota CAS: 10 Retries May Not Be Enough

Under extremely high contention (many concurrent requests for the same `appTag`), 10 CAS retries could be exhausted. When this happens, the request is rejected with `QuotaStateError` (HTTP 503). This is intentionally fail-closed — it's safer to reject than to bypass quota.

## Sender Cache: 10-Minute Staleness Window

The gateway caches sender configs for 10 minutes. After `mail-admin` updates a sender, the gateway may serve the old config for up to 10 minutes. `Put()` and `Delete()` invalidate the cache eagerly, but only on the instance that processed the write — other gateway instances still have the old cache entry.

This is an acceptable trade-off: sender configs change infrequently, and eventual consistency is fine.

## Spam Check: TTL Handles Expiry, No Explicit Delete

The `spam` KV bucket has a TTL (default 60 seconds, configurable via `DISPATCH_SPAM_TIMEOUT_SECONDS`). NATS automatically removes expired keys. The spam checker never explicitly deletes entries — it relies entirely on TTL.

This means the spam window is a sliding window: any duplicate within the TTL is blocked.

## spam.Checker: Get-Before-Put Is Not Atomic

The spam check does `Get(hash)` then `Put(hash)` — this is NOT an atomic check-and-set. Two concurrent requests with the same hash could both see "not found" and both proceed. This is a known trade-off: the spam check is a best-effort dedup, not a hard guarantee. The hard dedup layer is `delivered` KV in the worker.

## sender.Store.List() Reads All Keys Then All Values

`List()` calls `kv.Keys()` then iterates calling `Get()` for each key. Each `Get()` goes through the cache — so the cache is populated as a side effect. This is fine for small numbers of senders but would be slow for hundreds.

## Quota Data Structure: Timestamp Key Is NOT the NATS KV Timestamp

The quota entries have a `ts` field (Unix timestamp) set to `time.Now().Unix()` at write time. This is the application-level timestamp, not the NATS message timestamp. The 24-hour window is calculated from this application timestamp, not from NATS's internal timestamps.
