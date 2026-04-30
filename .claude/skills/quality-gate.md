---
name: quality-gate
description: Run lint + tests and report pass/fail with a compact summary
---

Run the quality gate for the dispatch project:

1. Run `devbox run lint` in the working directory `/Volumes/SSD2TB/work/antigravity/dispatch`. Capture all output.
2. Run `devbox run test` in the same directory. Capture all output.
3. Report results as a compact table:

| Check | Status | Details |
|-------|--------|---------|
| lint  | ✓ / ✗  | number of findings, or "clean" |
| test  | ✓ / ✗  | X passed, Y failed |

If anything failed, list each finding/failure with file:line and a one-line description.
Do not show raw tool output — only the summary table and the failure list.
End with a single line: "Quality gate PASSED" or "Quality gate FAILED — N issues".
