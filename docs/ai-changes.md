# AI Changes Log – dispatch

**Zweck:** Kurze, nachvollziehbare Dokumentation aller relevanten Änderungen durch Claude Code inklusive Begründung.  
**Format:** Immer strikt einhalten. Maximal 5 Zeilen pro Eintrag.  
**Pflicht:** Nach jeder nicht-trivialen Änderung einen Eintrag anhängen.

---

## Format-Vorgabe

```markdown
## YYYY-MM-DD — [Kurzer Task-Titel]

**Begründung:** [1–2 Sätze: Warum wurde das so gemacht? Welches Problem/Prinzip stand dahinter?]
**Änderungen:**
- `pfad/datei.go` (kurze Beschreibung der Änderung)
- `pfad/test.go` (neuer Testfall / Anpassung)
**Ergebnis:** `devbox run lint` + `devbox run test` → [Ergebnis]
**Hinweis:** [optional: offener Punkt, **WICHTIG**, **DESIGN-DECISION**, Follow-up]
```

---

## 2026-05-01 — Verbesserte CLAUDE.md + ai-changes Log eingeführt

**Begründung:** Ursprüngliche CLAUDE.md war zu lang (> 350 Zeilen). Nach Best-Practice-Empfehlungen auf ~140 Zeilen gekürzt und progressive Disclosure eingeführt. Neues Log für bessere Nachvollziehbarkeit und kontinuierliche Verbesserung der Guidelines.
**Änderungen:**
- `CLAUDE.md` (komplett neu strukturiert, gekürzt, neue Workflow-Regel)
- `docs/ai-changes.md` (neu angelegt mit Template + Beispielen)
**Ergebnis:** Keine Tests betroffen (Dokumentation).
**Hinweis:** **WICHTIG** – Ab sofort nach jeder Änderung Log-Eintrag pflichtig.

## 2026-05-01 — Dockerfile + .dockerignore für alle Services

**Begründung:** Ein einziges parametrisiertes Dockerfile (`ARG SERVICE`) statt vier separate — vermeidet Duplikation und hält Build-Logik an einem Ort. Distroless-Runtime-Image (4,9 MB, non-root, kein Shell) für minimale Angriffsfläche in AKS.
**Änderungen:**
- `Dockerfile` (multi-stage, `ARG SERVICE`, Builder golang:1.25-alpine, Runtime distroless/static-debian12:nonroot)
- `.dockerignore` (.git, .devbox, coverage-Artefakte ausgeschlossen)
**Ergebnis:** `docker build --build-arg SERVICE=mail-gateway` → Image 4,9 MB, Build erfolgreich.
**Hinweis:** `GOARCH=amd64` hardcoded — bei ARM-Deployments (Apple Silicon Nodes) `--platform linux/amd64` oder `GOARCH` per Build-Arg anpassen.
