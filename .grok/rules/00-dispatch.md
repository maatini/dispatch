# Grok-only notes for dispatch

Canonical project rules: root `CLAUDE.md` (all harnesses). This file is Grok-specific only.

## Prefer plan first when editing

Propose a short plan and wait for confirmation before changing fail-closed or delivery paths:

- `internal/worker/processor.go` (ACK, dedup Get/Put, MaxDeliver, DLQ)
- `internal/quota/`, `internal/spam/`
- `internal/gateway/handler.go` (quota/spam/auth/publish)
- `internal/natsutil/` consumer/stream provision (AckWait, MaxDeliver)

Trivial typo/doc fixes: no plan required.

## Tools

- Ignore lean-ctx / RTK if those tools are not available; use native file + shell tools.
- Prefer `devbox run test` / `devbox run lint` over bare `go test` outside devbox.
- Read-only exploration of large surfaces: use explore subagent when helpful; do not edit via subagent for invariant paths without review.

## Git

- No force-push to `main`. Confirm before `git push` unless the user already asked to push.
- Do not commit `.devbox/` generated hooks unless the user explicitly wants them.

## Continuity

- Skim `docs/ai-changes.md` **Hot decisions** + last 5 entries; do not re-read the full log.
- Product backlog: `improvements.md` only.
