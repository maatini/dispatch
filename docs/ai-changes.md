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

## 2026-05-01 — CI-Fix: security.yml Push-Trigger entfernt, CodeQL v4

**Begründung:** `security.yml` lief concurrent mit `build.yml` bei Push auf main — GHCR-Images existieren zu diesem Zeitpunkt noch nicht. Wöchentlicher Schedule reicht, da `build.yml` bereits jeden Build scannt.
**Änderungen:**
- `.github/workflows/security.yml` (Push-Trigger entfernt; `codeql-action/upload-sarif@v3` → `v4`)
- `.github/workflows/build.yml` (`codeql-action/upload-sarif@v3` → `v4`)
**Ergebnis:** Workflow-Änderung; kein lokaler Build betroffen.

## 2026-05-01 — Versionierung: internal/version + ldflags

**Begründung:** Einheitliche Versionsnummer 0.5.0 in allen Services, beim Start geloggt und via `-X dispatch/internal/version.Version` zur Build-Zeit injizierbar.
**Änderungen:**
- `internal/version/version.go` (neues Paket, `var Version = "0.5.0"`)
- `cmd/*/main.go` (alle 4 Services: `loggy.Kv("version", version.Version)` im Startup-Log)
- `Dockerfile` (`ARG VERSION=0.5.0`, ldflags `-X dispatch/internal/version.Version=${VERSION}`)
- `.github/workflows/build.yml` (`build-args: VERSION=0.5.0`)
**Ergebnis:** `go build ./cmd/...` + `go test ./...` → 151 Tests, alles grün.

## 2026-05-01 — GitHub Workflows: Docker-Build, Trivy-Scan, SBOM

**Begründung:** CI deckt jetzt den vollständigen Image-Lifecycle ab: Build → Vulnerability-Scan → SBOM → Push. Trivy SARIF-Upload ins GitHub Security-Tab ermöglicht Triage ohne externen Scan-Server.
**Änderungen:**
- `.github/workflows/build.yml` (neuer `docker`-Job: Matrix über 4 Services, Trivy SARIF + CycloneDX SBOM, GHCR-Push nur auf main)
- `.github/workflows/security.yml` (neuer `trivy`-Job: wöchentlicher Scan der GHCR-Images `:latest`, SARIF + SBOM)
**Ergebnis:** Workflow-Änderungen, keine lokalen Tests betroffen.
**Hinweis:** `security.yml` trivy-Job setzt voraus, dass `:latest`-Images bereits in GHCR existieren (erster Push via `build.yml` auf main nötig).

## 2026-05-01 — Dockerfile + .dockerignore für alle Services

**Begründung:** Ein einziges parametrisiertes Dockerfile (`ARG SERVICE`) statt vier separate — vermeidet Duplikation und hält Build-Logik an einem Ort. Distroless-Runtime-Image (4,9 MB, non-root, kein Shell) für minimale Angriffsfläche in AKS.
**Änderungen:**
- `Dockerfile` (multi-stage, `ARG SERVICE`, Builder golang:1.25-alpine, Runtime distroless/static-debian12:nonroot)
- `.dockerignore` (.git, .devbox, coverage-Artefakte ausgeschlossen)
**Ergebnis:** `docker build --build-arg SERVICE=mail-gateway` → Image 4,9 MB, Build erfolgreich.
**Hinweis:** `GOARCH=amd64` hardcoded — bei ARM-Deployments (Apple Silicon Nodes) `--platform linux/amd64` oder `GOARCH` per Build-Arg anpassen.

## 2026-07-17 — plan07: Audit-Fixes Phase 1+2 (Quick Wins #1–#8, #11 teilw.)

**Begründung:** Umsetzung von `plan07.md`: Quick-Win-Fixes aus `improvements.md` für Korrektheit unter Fehlerbedingungen plus echte Integrationstests; jede Aufgabe mit Regressions-Test.
**Änderungen:**
- `internal/msgraph` (Breaker-open → transient; Token-Client 15s-Timeout, 4xx permanent; `X-Dispatch-TraceId`-Header; NDR `toRecipients`/`receivedDateTime`)
- `internal/bounce`, `internal/admin`, `internal/worker`, `internal/spam`, `internal/gateway`, `internal/domain` (MarkAsRead nur bei Erfolg; ReprocessDeadLetter mit TraceID-Headern; Dedup via Payload-TraceID + DLQ bei fehlender; atomares Spam-KV-`Create`; echte Readiness; Fehlercodes `VALIDATION_FAILED`/`INTERNAL_ERROR`, Spam-State 503; `readStream` 5s-Timeout + ctx)
- `internal/{gateway,worker,admin}/integration_test.go` (5 Integrationstests hinter `//go:build integration`, Skip ohne NATS)
- `README.md` (Testzähler 151 → 171), `improvements.md` (Items #1–#8, #11 teilw. als erledigt markiert)
**Ergebnis:** `go build/vet/test -race` + `golangci-lint` → 171 Tests grün, 0 Issues; `go test -tags integration -race ./...` gegen lokales NATS → grün (A–E).
**Hinweis:** Items #9 (exp-Claim) und #10 (Sonar-Token) aus den Quick Wins sind nicht Teil von plan07 und bleiben offen.

## 2026-07-17 — CI: Trivy-Version auf v0.72.0 gepinnt

**Begründung:** GitHub-Notiz meldete verfügbare Trivy-Version 0.72.0 (bisher 0.70.0 via Action-Default); Pinning entspricht der Tool-Version-Pinning-Policy der Pipeline.
**Änderungen:**
- `.github/workflows/build.yml` (`version: v0.72.0` in beiden `trivy-action`-Steps)
- `.github/workflows/security.yml` (`version: v0.72.0` in beiden `trivy-action`-Steps)
**Ergebnis:** Workflow-Änderung; kein lokaler Build betroffen.
