# Shared Patterns

Reusable conventions and Go idioms used across multiple modules.

## Context as First Parameter

Every function that does I/O or blocking work takes `context.Context` as its first parameter. Context is never stored in structs — always passed explicitly.

```go
// Correct
func (p *Processor) Handle(ctx context.Context, msg *nats.Msg) { ... }

// Never
type Processor struct {
    ctx context.Context // ← wrong
}
```

## Consumer-Side Interfaces

Interfaces are defined where they are **used**, not where they are **implemented**. The consumer declares only the methods it actually needs.

```go
// gateway/handler.go — defines only what it needs
type senderLookup interface {
    Get(appTag string) (domain.Sender, error)
}

// sender/sender.go — Store has many more methods
type Store struct { ... }
func (s *Store) Get(appTag string) (domain.Sender, error) { ... }
func (s *Store) Put(sender domain.Sender) error { ... }
func (s *Store) Delete(appTag string) error { ... }
func (s *Store) List() ([]domain.Sender, error) { ... }
```

## Error Wrapping with Context

All errors are wrapped with context using `fmt.Errorf("operation: %w", err)`. Use `errors.Is()` and `errors.As()` for inspection.

```go
if err := h.nats.Publish(ctx, msg); err != nil {
    return fmt.Errorf("publish to NATS: %w", err)
}

// Inspection
var transient *msgraph.GraphTransientError
if errors.As(err, &transient) { ... }
```

## Graceful Shutdown

All 4 services use the same shutdown pattern:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
// ... start server or consumer ...
<-ctx.Done()
// ... shutdown ...
```

## PII Masking in Logs

Every log statement that includes an email address must use `pii.MaskEmail()`:

```go
handlerLog.Warnc(ctx, loggy.CategoryBusinessRuleViolation, "domain not whitelisted",
    loggy.Kv("recipient", pii.MaskEmail(addr)),
)
```

## Package-Level Logger Instances

Each file declares its own package-level logger:

```go
var handlerLog = loggy.GetLogger("Handler")
var procLog = loggy.GetLogger("Processor")
var clientLog = loggy.GetLogger("MSGraphClient")
```

## Context-Enriched Loggers

For request-scoped logging, create a derived logger:

```go
log := procLog.With(loggy.Kv("traceId", traceID))
log.Info("processing mail")
```

## NATS KV with Optimistic CAS

When updating NATS KV values that could be concurrently modified, use the revision number for CAS:

```go
kve, _ := c.kv.Get(appTag)
revision := kve.Revision()
// ... modify value ...
_, err = c.kv.Update(appTag, data, revision)
// If err is JetStreamError → CAS conflict → retry
```

## Binaries: Identical Startup Sequence

All 4 `cmd/*/main.go` files follow the same pattern:
1. `config.Load()` — load env vars
2. `natsutil.Connect(cfg.NatsURL)` — connect to NATS
3. `natsutil.ProvisionStreams(js)` / `ProvisionKVBuckets` / `ProvisionObjectStore` — ensure infrastructure
4. Create domain services/client with configuration
5. Start server/consumer/ticker
6. `signal.NotifyContext` for graceful shutdown
