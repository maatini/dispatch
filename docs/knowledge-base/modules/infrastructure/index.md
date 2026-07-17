# infrastructure

Cross-cutting utility packages with no domain logic: structured logging (loggy), NATS setup/constants (natsutil), SHA-256 hashing (hash), and PII masking (pii).

Source: `internal/loggy/` + `internal/natsutil/` + `internal/hash/` + `internal/pii/`

## Files

- **[responsibility.md](responsibility.md)** — What each sub-package owns: logging, NATS resource provisioning, hashing, PII masking
- **[dependencies.md](dependencies.md)** — Outbound to Go stdlib + nats.go; used by all other modules
- **[interfaces.md](interfaces.md)** — Logger API, NATS subject/stream names, SpamHash/MaskEmail signatures
- **[gotchas.md](gotchas.md)** — Never use slog directly, stream provisioning is idempotent, log categories matter
