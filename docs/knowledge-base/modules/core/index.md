# core

Domain types, configuration loading, and version. Everything in the system depends on these packages — they have no internal dependencies themselves.

Source: `internal/domain/` + `internal/config/` + `internal/version/`

## Files

- **[responsibility.md](responsibility.md)** — Domain model ownership, error type hierarchy, config contract, version injection
- **[dependencies.md](dependencies.md)** — Zero internal deps (leaf); everything depends on this
- **[interfaces.md](interfaces.md)** — All domain types, error types, Config struct, Version var
- **[gotchas.md](gotchas.md)** — MailRequest vs MailRequestDO distinction, ApiError wrapping, zero-value quota semantics
