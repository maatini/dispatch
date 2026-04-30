---
name: coverage
description: Run tests with coverage and show only packages below threshold
---

Run coverage analysis for the dispatch project:

1. Run in `/Volumes/SSD2TB/work/antigravity/dispatch`:
   ```
   go test -coverprofile=/tmp/dispatch-cov.out -covermode=atomic ./...
   go tool cover -func=/tmp/dispatch-cov.out
   ```
2. Parse the per-function output. Group by package.
3. Compute per-package statement coverage (average of all functions in the package).
4. The threshold is **80%** for core packages: `internal/gateway`, `internal/quota`, `internal/spam`, `internal/worker`, `internal/pii`, `internal/hash`, `internal/config`, `internal/sender`.

Report two sections:

**Below threshold (< 80%)** — table with package, coverage %, and the specific uncovered functions.
**Above threshold** — one-liner: "X packages at ≥ 80%".

If args were passed (e.g. `/coverage 70`), use that number as the threshold instead.

End with the total statement coverage across all packages.
