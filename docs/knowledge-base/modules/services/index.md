# services

Domain services backed by NATS KV buckets: quota enforcement (rolling 24h with optimistic CAS), sender configuration (KV store with in-memory cache), and spam deduplication (SHA-256 with TTL bucket).

Source: `internal/quota/` + `internal/sender/` + `internal/spam/`

## Files

- **[responsibility.md](responsibility.md)** — What each service owns: quota CAS loop, sender cache, spam TTL check
- **[dependencies.md](dependencies.md)** — Inbound (gateway/admin) and outbound (NATS KV, domain types)
- **[interfaces.md](interfaces.md)** — Function signatures, KV store interfaces, error contracts
- **[gotchas.md](gotchas.md)** — CAS retry exhaustion, cache TTL staleness, TTL vs explicit delete
