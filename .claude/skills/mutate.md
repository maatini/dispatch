---
name: mutate
description: Run gremlins mutation tests and summarise results per package
---

Run mutation tests for the dispatch project.

The packages under test are:
- `./internal/gateway`
- `./internal/quota`
- `./internal/spam`
- `./internal/worker`
- `./internal/loggy`

If args were passed (e.g. `/mutate ./internal/quota`), run only that package instead.

For each package, run in `/Volumes/SSD2TB/work/antigravity/dispatch`:
```
go tool gremlins unleash --workers 4 --timeout-coefficient 20 <package>
```

After all runs, report a summary table:

| Package | Killed | Lived | Not covered | Timed out | Score | Threshold |
|---------|--------|-------|-------------|-----------|-------|-----------|
| gateway | N      | N     | N           | N         | N%    | ✓ / ✗     |
| ...     |        |       |             |           |       |           |

Threshold is **70%** for both efficacy and mutation-coverage (from `.gremlins.yaml`).

For any LIVED mutant, show:
- file:line — what the mutant changed — "no test asserts this behaviour"

For any NOT COVERED mutant, show:
- file:line — which code path is not reached by any test

End with: "Mutation gate PASSED" or "Mutation gate FAILED — list which packages failed and why".
