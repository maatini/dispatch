# services (quota / sender / spam)

**Source:** `internal/quota/`, `internal/sender/`, `internal/spam/`  
**Role:** NATS-KV-backed domain services shared by gateway (and admin for sender).

## quota.Checker

- Rolling 24h window of per-request count entries; optimistic CAS (max 10 retries, exponential pause + jitter between conflicts).
- `JetStreamError` (CAS conflict) → retry; other errors → `QuotaStateError` fail-closed.
- Exhausted retries → 503 (safer than bypass).

## sender.Store

- KV + 30s in-memory cache; Put/Delete invalidate local entry only. Cross-process staleness up to TTL.

## spam.Checker + Hash

- `spam.Hash(appTag, subject, recipients, bodyLen, htmlLen)` — SHA-256 fingerprint (recipients order as in request).
- `Check(hash)` — atomic `kv.Create`; `ErrKeyExists` → spam validation error; other → `SpamStateError` fail-closed.
- Bucket TTL (default 60s) handles expiry; no explicit delete.

Do not merge these three packages — different fail-closed semantics and TTLs.
