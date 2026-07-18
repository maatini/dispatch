# infrastructure

**Source:** `internal/loggy/`, `internal/natsutil/`, `internal/httpsrv/`, `internal/testkit/`

## loggy

- **Only** logging path in production code — never `slog.*` / `fmt.Print*` directly.
- `GetLogger(name)` → independent JSON handler on stdout (not `slog.Default()`).
- PII: `loggy.MaskEmail(addr)` for every email in logs.

## natsutil

- `Connect` — reconnect retries built in.
- `Setup(js, spamTTL)` — idempotent streams + KV (all four binaries call this).
- `ProvisionObjectStore`, `ProvisionWorkerConsumer(js, ackWait, maxDeliver)` — create **and update** consumer.
- Constants: streams `DISPATCH_*`, buckets `senders`/`quota`/`spam`/`delivered`, subjects `cody.mailing.*`.

## httpsrv / testkit

- Shared graceful HTTP server lifecycle for gateway + admin.
- `testkit` — mock KV and helpers for unit tests without real NATS.

## Gotchas

- Prefer `Setup()` over ad-hoc provision in new binaries.
- Consumer must be provisioned before worker `PullSubscribe`.
- Concurrent service starts racing on stream create are OK (upsert).
