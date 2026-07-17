# msgraph

Microsoft Graph API client abstraction. Handles OAuth2 token acquisition, circuit breaking, retry logic, rate limiting, email sending (inline + upload session), and bounce mailbox polling.

Source: `internal/msgraph/`

## Files

- **[responsibility.md](responsibility.md)** — What this module owns: MS Graph HTTP client, auth, resilience, send + bounce operations
- **[dependencies.md](dependencies.md)** — Outbound (MS Graph REST API, OAuth2 endpoint) — this is a leaf module with no internal deps upward
- **[interfaces.md](interfaces.md)** — Service.SendEmail signature, error types, Client construction
- **[gotchas.md](gotchas.md)** — Circuit breaker tuning, token cache buffer, dev proxy mode, inline vs upload session threshold
