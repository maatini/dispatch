# Maintenance

How and when to update this knowledge base.

## When to Update

Update the knowledge base when any of these change:

- **New package added** → Add a new module folder under `modules/` or add to an existing module
- **Module boundary changes** → Update the relevant module's `responsibility.md` and `dependencies.md`; update `architecture/components.md`
- **NATS topology changes** (new stream, KV bucket, subject, consumer) → Update `architecture/dependencies.md` (resource ownership matrix) and `modules/infrastructure/interfaces.md` (name constants)
- **New service/binary** → Add to `overview.md`, `architecture/components.md`, and create a new module folder
- **Error semantics change** → Update the affected module's `gotchas.md` and `interfaces.md`
- **New cross-cutting pattern** → Add to `cross-cutting/shared-patterns.md`
- **New architectural decision** → Add to `architecture/decisions.md`

## How to Update

1. **Keep it signal-dense.** Each file should be scannable in under 60 seconds. Use bullets, tables, and short paragraphs.
2. **Update index.md links.** Every file added or removed must be reflected in the module's `index.md`.
3. **Verify Mermaid diagrams.** Paste any changed Mermaid blocks into a Mermaid live editor to confirm they render.
4. **Ground everything in code.** When describing a responsibility or interface, reference the actual Go file and function name.
5. **Mark inferences clearly.** If you're not sure about something, note it as "Needs clarification" or "Inferred from X" — don't invent.

## What NOT to Do

- Don't duplicate information that exists in `ARCHITECTURE.md` or `README.md` — link to them instead
- Don't add speculation, future plans, or "we might want to" content
- Don't write narrative essays — keep it scannable
- Don't include code snippets longer than ~10 lines — reference the source file instead

## Relationship to Other Docs

| Doc | Role | KB Relationship |
|---|---|---|
| `README.md` | User-facing overview, API docs, config | KB links to README for API details |
| `ARCHITECTURE.md` | Detailed architecture (German) | KB is the agent-facing English summary; links to ARCHITECTURE.md for deep dives |
| `CLAUDE.md` | AI behavioral guidelines, invariants | KB is referenced from CLAUDE.md as the architecture authority |
| `docs/ai-changes.md` | AI change log (German) | KB's `architecture/decisions.md` captures stable decisions; ai-changes.md captures recent changes |

## Quick Reference: Module File Purposes

| File | Purpose | When to Update |
|---|---|---|
| `index.md` | Summary + links | Any file added/removed in the module |
| `responsibility.md` | What this owns, invariants, entry points | When ownership or invariants change |
| `dependencies.md` | Inbound/outbound deps + graph | When dependencies change (new import, new consumer) |
| `interfaces.md` | Public API surface | When function signatures, HTTP endpoints, or NATS subjects change |
| `gotchas.md` | Pitfalls and edge cases | When you discover a new gotcha or fix one that caused a bug |
