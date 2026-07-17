# services: Interfaces

## quota.Checker

```go
type Checker struct { ... }

func NewChecker(kv nats.KeyValue) *Checker

// Check verifies and records recipient usage.
// Returns nil on success, QuotaError if exceeded, QuotaStateError if KV unavailable.
func (c *Checker) Check(appTag string, limit, requested int) error

// CurrentUsage returns the rolling 24h recipient count (for X-RateLimit-* headers).
func (c *Checker) CurrentUsage(appTag string) (int, error)
```

**KV store interface (consumer-side):**
```go
type kvStore interface {
    Get(key string) (nats.KeyValueEntry, error)
    Create(key string, value []byte) (uint64, error)
    Update(key string, value []byte, last uint64) (uint64, error)
}
```

## sender.Store

```go
type Store struct { ... }

func New(kv nats.KeyValue, cacheTTL time.Duration) *Store
func (s *Store) Get(appTag string) (domain.Sender, error)
func (s *Store) Put(sender domain.Sender) error
func (s *Store) Delete(appTag string) error
func (s *Store) List() ([]domain.Sender, error)
func (s *Store) InvalidateCache(appTag string)
```

**KV store interface (consumer-side):**
```go
type kvStore interface {
    Get(key string) (nats.KeyValueEntry, error)
    Put(key string, value []byte) (uint64, error)
    Create(key string, value []byte) (uint64, error)
    Delete(key string, opts ...nats.DeleteOpt) error
    Keys(opts ...nats.WatchOpt) ([]string, error)
}
```

## spam.Checker

```go
type Checker struct { ... }

func NewChecker(kv nats.KeyValue) *Checker

// Check returns ValidationError if the hash was seen within the bucket TTL.
// Otherwise records the hash and returns nil.
func (c *Checker) Check(hash string) error
```

**KV store interface (consumer-side):**
```go
type kvStore interface {
    Get(key string) (nats.KeyValueEntry, error)
    Put(key string, value []byte) (uint64, error)
}
```

## Error Contracts

| Method | Error Type | HTTP Mapping (gateway) |
|---|---|---|
| `quota.Check()` → exceeded | `*domain.QuotaError` | 429 + `X-RateLimit-*` |
| `quota.Check()` → KV failure | `*domain.QuotaStateError` | 503 |
| `sender.Get()` → unknown | `*domain.ValidationError{Code: ErrUnknownAppTag}` | 400 |
| `spam.Check()` → duplicate | `*domain.ValidationError{Code: ErrSpamDetected}` | 400 |
