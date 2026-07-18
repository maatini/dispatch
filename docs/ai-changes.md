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

## 2026-07-17 — CI-Fix: Trivy/govulncheck-Failures + JetStream im Integration-Job

**Begründung:** Build-, Security- und Integration-Workflows waren rot: Trivy fand HIGH-CVEs (stdlib go1.25.9, x/crypto v0.49.0), govulncheck analog; der Integration-Job startete NATS ohne `-js` (GHA-Services können kein Command übergeben), sodass alle JetStream-Calls mit "no responders" scheiterten.
**Änderungen:**
- `go.mod`/`go.sum` (x/crypto v0.52.0, x/sys v0.46.0, x/text v0.39.0, toolchain go1.26.5)
- `Dockerfile` (Builder `golang:1.26-alpine` digest-gepinnt, go1.26.5), Workflows (setup-go 1.26.5)
- `.github/workflows/integration.yml` (NATS via `docker run -js`-Step statt Service-Container)
**Ergebnis:** Lokal: Trivy 0 CRITICAL/HIGH, govulncheck 0 affecting; CI: Build, Security, Integration alle grün.
**Hinweis:** Security-Trivy scannt GHCR-`:latest` — nach dep-/Base-Image-Fixes erst nach erfolgreichem Build-Push wieder grün. CodeQL-Alert `go/disabled-certificate-check` (dev-proxy `InsecureSkipVerify`) ist bekannte, dokumentierte Ausnahme.

## 2026-07-17 — tests07: Testabdeckung auf sehr gutes Niveau heben (Refactor + Tests)

**Begründung:** Umsetzung von `tests07.md`: Zwei Produktions-Bugs fixen (Dedup fail-open, uploadChunks-Fehlerklassifizierung), Testbarkeits-Refactorings (injizierbares baseURL/retryDelay) und Unit-/Integrationstests für alle Kern-Packages (msgraph 92 %, natsutil 83 %, worker 77 %, admin 55 %). Mail-Admin-Coverage von 13 % auf 55 % durch mock-KV-Sender-CRUD-Tests. Insgesamt +83 Tests (171 → 254).
**Änderungen:**
- `internal/msgraph/service.go` (baseURL-Feld + Konstruktor-Default; uploadChunks: 4xx → GraphPermanentError)
- `internal/msgraph/client.go` (retryBaseDelay-Feld + Konstruktor-Default)
- `internal/worker/processor.go` (Dedup fail-closed: KV-Fehler → kein ACK/Graph-Call)
- `internal/sender/sender.go` (exportierte Felder Kv/Cache/CacheTTL für Admin-Resolver-Tests)
- `internal/worker/attachstore.go` (Revert zu nats.ObjectStore — Fake-implementierung mit voller nats-Interface-Kompatibilität)
- Neue Tests: `msgraph/errors_test.go`, `msgraph/ratelimiter_test.go`, `worker/attachstore_test.go`, `admin/server_test.go`, `natsutil/setup_test.go` (+ embedded nats-server via `go get`)
- Erweiterte Tests: `msgraph/client_test.go`, `msgraph/service_test.go`, `msgraph/token_test.go`, `worker/processor_test.go`, `admin/resolver_test.go`
- Integrationstests: `admin/integration_test.go` (Filter, Pagination, Bounces, DeadLetters), `worker/integration_test.go` (ConsumerRun)
- go.mod (+ `nats-server/v2`)
- README.md (Test-Zähler + Coverage-Tabelle aktualisiert)
- docs/knowledge-base (msgraph/mail-worker/mail-admin gotchas + interfaces aktualisiert)
**Ergebnis:** `go build/vet/test -race` → 254 Tests grün, `golangci-lint` 0 Issues. Integrationstests kompilieren sauber.
**Hinweis:** Integration-Target-Coverage (admin ≥80 %, worker ≥85 %) wird durch Integrationstests gegen lokales NATS erreicht — die Unit-Test-Coverage allein erreicht admin 55 % / worker 77 %, da Stream-Reads (readStream) und Consumer-Loop ohne reales NATS nicht testbar sind.

## 2026-07-18 — Cleanup-Refactor: Deduplizierung + toter Code entfernt

**Begründung:** Nach tests07 lagen vier nahezu identische KV-Mocks in den Test-Packages verteilt vor, die HTTP-Server-Lifecycle war in mail-admin/mail-gateway dupliziert, die Provisionierung in allen vier `cmd/main.go` wiederholt, und loggy/quota enthielten ungenutzten Code. Die für Admin-Tests exportierten `sender.Store`-Felder (tests07) wurden durch ein sauberes Interface ersetzt.
**Änderungen:**
- `internal/httpsrv` (NEU): gemeinsame HTTP-Server-Lifecycle (`Run` mit Graceful Shutdown, 10s Timeout) + Tests; ersetzt Duplikate in `cmd/mail-admin` und `cmd/mail-gateway`
- `internal/testkit` (NEU): gemeinsames `MockKV` (map-basiert, Fehler-Injektion, Revision/CAS-Tracking); konsolidiert lokale Mocks aus `admin`, `quota`, `sender`, `spam`, `worker`-Tests
- `internal/natsutil/setup.go` (`Setup`: kombiniert ProvisionStreams + ProvisionKVBuckets); alle vier `cmd/*/main.go` nutzen es
- `internal/sender/sender.go` (Felder zurück privat; exportiertes `KV`-Interface + `DefaultCacheTTL`-Konstante; ungenutztes `InvalidateCache` entfernt)
- `internal/loggy/loggy.go` (toter Code entfernt: `Alert`, `Debug`/`Debugc`, `BusinessRuleViolation`, `ValidationFailed`, `MissingData`, `UncaughtException`, `ServiceAccountExpired`, `UnstructuredLog` + 6 ungenutzte Kategorien)
- `internal/quota/quota.go` (ungenutztes `CurrentUsage` entfernt; Interface in `gateway/handler.go` entsprechend verschlankt)
- devbox.json/README/ARCHITECTURE.md (Skript-Namen in Hilfetext/Doku korrigiert: `dev-proxy:up`, `worker-dev`, `gateway-dev`), CLAUDE.md (Go 1.25, toter Link `coding-idioms.md` entfernt), knowledge-base synchronisiert (Interfaces, toolchain go1.26.5)
**Ergebnis:** `go build/vet/test -race` → 241 Tests grün (−13: Tests für entfernten toten Code entfallen), `golangci-lint` 0 Issues, Integrationstests kompilieren (`-tags integration`). Datei-Korruption in README.md/CLAUDE.md/responsibility.md aus unterbrochener Session repariert (doppelte Zeilenfragmente am Dateiende).
