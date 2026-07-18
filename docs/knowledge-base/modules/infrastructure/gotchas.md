# infrastructure: Gotchas

## NEVER Use slog or fmt.Println Directly

The most important rule in the entire codebase. Every log statement must go through `loggy`. Direct `slog.*` or `fmt.Print*` calls break:
- Structured JSON output format
- Semantic category tagging
- PII masking guarantees
- Log attribution (component name)

## Prefer Setup() in Binaries

`natsutil.Setup(js, spamTTL)` is the canonical entry for stream + KV provisioning. All four `cmd/*/main.go` call `Setup` then (where needed) `ProvisionObjectStore` / `ProvisionWorkerConsumer`. Prefer `Setup` over calling `ProvisionStreams` + `ProvisionKVBuckets` separately in new binaries.

## Stream Provisioning Is Idempotent but Not Atomic

`Setup()` / `ProvisionStreams()` and friends check if resources exist, then create or update. If two services start simultaneously, one's "create" may race with the other's "exists" check. In practice this is fine because:
- NATS `AddStream` for an existing stream returns an error that is ignored by `upsertStream`
- Subsequent starts always succeed

## loggy.GetLogger Creates an Independent slog.Logger

Each `GetLogger("ComponentName")` creates its own `slog.Logger` with `slog.NewJSONHandler(os.Stdout, nil)` — it does NOT depend on `slog.Default()`. This means `slog.SetDefault()` is not needed and loggy instances are fully independent.

## SpamHash Input Must Be Deterministic

The hash input format is `appTag|subject|recip1,recip2|bodyLen|htmlLen`. The recipients are joined in the order they appear in the slice — if the same set of recipients is passed in different orders, different hashes will be produced. This is by design: the gateway always passes recipients in the order they appear in the request.

## MaskEmail Handles Edge Cases

- `"a@b.com"` → `"a***@b.com"` (single char local part)
- `""` → `"***"` (no @ sign)
- `"user@domain.com"` → `"u***@domain.com"` (normal case)

## NATS Connection Has Retry Built In

`natsutil.Connect()` configures `RetryOnFailedConnect(true)`, `MaxReconnects(10)`, `ReconnectWait(2*time.Second)`. If NATS is down at startup, the connection will keep retrying. This means the service may start but block on the first NATS operation until the connection succeeds.

## Durable Consumer Must Be Provisioned Before Worker Starts

`ProvisionWorkerConsumer()` creates the `mail-worker` durable consumer on the `DISPATCH_MAILS` stream. If the worker starts before this is called, `PullSubscribe` will fail because the consumer doesn't exist. The `main.go` files handle this by calling provisioning before starting the consumer.
