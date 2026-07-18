# Maintenance

## When to update

| Change | Update |
|--------|--------|
| Module behavior / gotcha | `modules/<name>.md` |
| Stable architectural decision | `decisions.md` |
| New @tag or coding pattern | `cross-cutting/` |
| Pipeline / NATS topology diagrams | **root `ARCHITECTURE.md`** (not duplicated here) |
| Recent AI edits | `docs/ai-changes.md` |

## Rules

1. Keep each module file scannable in under a minute.
2. Do **not** re-copy pipeline diagrams from `ARCHITECTURE.md` — link instead.
3. Ground gotchas in real code paths; mark uncertainty explicitly.
4. One file per module under `modules/` (no multi-file folders).

## Related docs

| Doc | Role |
|-----|------|
| `README.md` | User API & config |
| `ARCHITECTURE.md` | Full pipeline (DE) |
| `CLAUDE.md` | AI guidelines + invariants |
| `docs/ai-changes.md` | Change log |
| `improvements.md` | Open backlog |
