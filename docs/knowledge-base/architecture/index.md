# Architecture

Global architecture documentation for the Dispatch system.

## Files

- **[components.md](components.md)** — High-level map of major logical components (4 services + 4 supporting layers), each with responsibilities and key files
- **[dependencies.md](dependencies.md)** — Complete dependency graph with Mermaid diagram, NATS resource ownership matrix, external dependency table
- **[data-flows.md](data-flows.md)** — Key data flows as Mermaid sequence diagrams (happy path: send mail, bounce detection)
- **[decisions.md](decisions.md)** — Key architectural decisions and their rationale (ADR-equivalent, sourced from `ARCHITECTURE.md` and `docs/ai-changes.md`)
