# services (quota / sender / spam)

**Source:** `internal/quota/`, `internal/sender/`, `internal/spam/`  
**Role:** NATS-KV-backed domain services shared by gateway (and admin for sender).

## quota.Checker

- Rolling 24h window of per-request count entries; optimistic CAS (max 10 retries).
- `JetStreamError` (CAS conflict) → retry; other errors → `QuotaStateError` fail-closed.
- Exhausted retries → 503 (safer than bypass).

## sender.Store

- KV + 10 min in-memory cache; Put/Delete invalidate local entry only.
- Cross-replica staleness up to TTL (acceptable; #16 backlog for KV watch).

## spam.Checker + Hash

- `spam.Hash(appTag, subject, recipients, bodyLen, htmlLen)` — SHA-256 fingerprint (recipients order as in request).
- `Check(hash)` — atomic `kv.Create`; `ErrKeyExists` → spam validation error; other → `SpamStateError` fail-closed.
- Bucket TTL (default 60s) handles expiry; no explicit delete.

Do not merge these three packages — different fail-closed semantics and TTLs.
