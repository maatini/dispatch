# infrastructure

Cross-cutting utility packages with no domain logic: structured logging (loggy), NATS setup/constants (natsutil), shared HTTP lifecycle (httpsrv), test doubles (testkit), SHA-256 hashing (hash), and PII masking (pii).

Source: `internal/loggy/` + `internal/natsutil/` + `internal/httpsrv/` + `internal/testkit/` + `internal/hash/` + `internal/pii/`

## Files

- **[responsibility.md](responsibility.md)** — What each sub-package owns: logging, NATS resource provisioning, HTTP server lifecycle, hashing, PII masking
- **[dependencies.md](dependencies.md)** — Outbound to Go stdlib + nats.go; used by all other modules
- **[interfaces.md](interfaces.md)** — Logger API, NATS subject/stream names, httpsrv.Run, testkit.MockKV, SpamHash/MaskEmail signatures
- **[gotchas.md](gotchas.md)** — Never use slog directly, stream provisioning is idempotent, log categories matter
