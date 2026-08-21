# AI Changes Log – dispatch

**Zweck:** Kurze, nachvollziehbare Dokumentation aller relevanten AI-Änderungen inklusive Begründung.  
**Format:** Immer strikt einhalten. Maximal 5 Zeilen pro Eintrag-Body.  
**Pflicht:** Nach jeder nicht-trivialen Änderung einen Eintrag **anhängen**.  
**Lesen:** Agents skim **Hot decisions** + last **5** entries only — full log only for forensics.

---

## Hot decisions (max ~10; update when MUST/ADR changes)

- 2026-08-21: `/metrics` unauthenticated (wie Health); `traceContext` nur W3C `traceparent`/`tracestate`; Graph `client-request-id` = dispatch traceId
- 2026-08-21: Worker **Fetch(1)** (AckWait nur für in-flight msg); KV-Bucket **TTL reconcile** on Setup; sender cache **30s**
- 2026-07-18: delivered KV **Get and Put** fail-closed (Put fail → no ACK; double-send worse than redelivery)
- 2026-07-18: Worker **AckWait 5m / MaxDeliver 8 / InProgress**; Dedup Get **before** MaxDeliver gate
- 2026-07-18: Gateway Bearer AuthN on `/mail/send`; Admin JWT **exp required**
- 2026-07-18: NATS-only state; no PostgreSQL/Redis/SQLite
- 2026-07-18: Spam fingerprint via `spam.Hash`; PII via `loggy.MaskEmail` (no separate hash/pii packages)
- 2026-07-18: KB = one file per module; diagrams only in root `ARCHITECTURE.md`

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

## 2026-07-18 — Doku & Knowledge-Base an Cleanup-Stand angeglichen

**Begründung:** Living Docs zeigten nach dem Cleanup noch tote Pfade (`docs/architecture.md`, `coding-idioms.md`), entfernte APIs (loggy-Semantikmethoden, `CurrentUsage`, `InvalidateCache`) und widersprüchliche Testzahlen (Badge 254 vs. 241).
**Änderungen:**
- `CLAUDE.md` (Pointer → `ARCHITECTURE.md` + `docs/knowledge-base/`; Projektstruktur ohne tote Dateien)
- `README.md` (Badge tests-241, konsistent mit Tabelle und `go test`)
- `ARCHITECTURE.md` (Log-Kategorien auf aktuelle 8 Konstanten; Spam-KV 503 ergänzt)
- `docs/knowledge-base/` (infrastructure: loggy/natsutil/`Setup`/httpsrv/testkit; services: Quota/Sender/Spam-Oberflächen + dependencies ohne loggy-Kanten + SpamStateError; shared-patterns Startup; mail-admin gotchas; architecture dependencies/components)
**Ergebnis:** Doc-only; Verifikation: 241 Unit-Tests, Grep toter Pfade/APIs = 0 in Living Docs, alle CLAUDE/KB-Links existieren.

## 2026-07-18 — P0: Send-Auth, JWT exp, delivered-Put fail-closed

**Begründung:** analyse07 P0 / `umstellung-p0.md`: unauthentifiziertes Send, Admin-JWT ohne exp ewig gültig, delivered-KV-Put-Fehler nach Graph-Success → ACK → Double-Send-Risiko. Double-Send schlimmer als Redelivery.
**Änderungen:**
- `internal/admin/auth.go` + `auth_test.go` (`jwt.WithExpirationRequired`; Token ohne exp → 401)
- `internal/worker/processor.go` + `processor_test.go` (Put-Fehler → kein Ack, kein Attachment-Cleanup; Test-Mode analog)
- `internal/gateway/handler.go` + Tests (Bearer `DISPATCH_GATEWAY_AUTH_TOKEN`, Health ohne Auth); `cmd/mail-gateway` fail-closed ohne Token außer `DISPATCH_GATEWAY_AUTH_DISABLED=true`
- `internal/config`, `domain.ErrUnauthorized`, README/Bruno/docker-compose
**Ergebnis:** `devbox run lint` 0 Issues; `devbox run test` grün.
**Hinweis:** **WICHTIG** / breaking: Clients brauchen `Authorization: Bearer …` am Send-Endpunkt; Admin-Tokens ohne `exp` sterben; shared Load erfordert Gateway-Token nicht (nur mail-gateway Startup).

## 2026-07-18 — Cleanup: veraltete Pläne/Doku entfernt, KB an P0 angeglichen

**Begründung:** Abgeschlossene Plan-/Audit-Dateien und stale Gotchas widersprachen dem Tree nach P0 (Auth, JWT exp, delivered-Put).
**Änderungen:**
- gelöscht: `plan07.md`, `tests07.md`, `analyse07.md`, `umstellung-p0.md`
- `improvements.md` auf offenes Backlog (#12–#20, #14b) reduziert; `kladde.md` ohne Secret
- KB/ARCHITECTURE: Gateway-Auth, JWT `WithExpirationRequired`, delivered-Put fail-closed
**Ergebnis:** Doc-only; Living Docs deckungsgleich mit P0-Code.
**Hinweis:** Historische Verweise in älteren `ai-changes`-Einträgen auf gelöschte Pläne bleiben als Audit-Trail.

## 2026-07-18 — #13 Worker AckWait 5m / MaxDeliver 8 / InProgress

**Begründung:** Redelivery-Races bei langer Graph/Attachment-Arbeit (AckWait 30s ohne Heartbeat) und Poison-Loops (`MaxDeliver: -1`) schließen; Zero-Double-Delivery und fail-closed bleiben.
**Änderungen:**
- `internal/config` (`WorkerAckWait`/`WorkerMaxDeliver`; Env invalid/≤0 bzw. MaxDeliver&lt;1 → Defaults 5m/8; infinite verboten)
- `internal/natsutil.ProvisionWorkerConsumer(js, ackWait, maxDeliver)` create **+ UpdateConsumer** reconcile
- `internal/worker.Processor` InProgress-Heartbeat; MaxDeliver-Gate nach Dedup Get → DLQ+FAILED+Term; `cmd/mail-worker` wired
- Tests: config env, consumer create/update, MaxDeliver boundary + dedup-wins; integration MaxDeliver→DLQ
- Living Docs: `improvements.md` #13 → Umgesetzt; ARCHITECTURE/KB/README/CLAUDE
**Ergebnis:** `devbox run lint && devbox run test` → grün.
**Hinweis:** **DESIGN-DECISION** Dedup vor MaxDeliver-Gate; **WICHTIG** Worker-Restart nötig damit bestehende Cluster UpdateConsumer mit 5m/8 bekommen.

## 2026-07-18 — Vereinfachung: Mikro-Packages + KB-Verdichtung

**Begründung:** Go-Code war schlank; Komplexität vor allem in 8×5 KB-Dateien und 1-Funktion-Packages (`hash`, `pii`). Invarianten unangetastet.
**Änderungen:**
- `spam.Hash` (ex `hash.SpamHash`); `loggy.MaskEmail` (ex `pii`); Packages `internal/hash`, `internal/pii` entfernt
- `docs/knowledge-base/`: Module je 1 Datei; `architecture/*` Diagramme → nur noch Root-`ARCHITECTURE.md`; ~2,9k → ~0,5k Zeilen KB
- CLAUDE/README/devbox mutate/skills an neue Pfade angepasst
**Ergebnis:** `devbox run lint` 0 Issues; `devbox run test` grün.
**Hinweis:** Keine Verhaltensänderung an HTTP/NATS/Graph.

## 2026-07-18 — Agent-Steuerung: CLAUDE v2.5 + Hot decisions

**Begründung:** Bewertung Systemprompt/Projekt-Rules (Score ~78): Domain stark, Harness-Noise (RTK/lean-ctx) und fehlender Task-Router schwächen Multi-Agent-Nutzung. Tier-A Docs-only.
**Änderungen:**
- `CLAUDE.md` v2.5 (§0 harness notes, §9 task router, Operability, slim §5→shared-patterns)
- `docs/ai-changes.md` (Hot decisions Kopf; Read-last-5 Regel)
- `docs/knowledge-base/maintenance.md` (CLAUDE/Hot-decisions Pflege)
**Ergebnis:** Doc-only; kein Build/Test-Lauf nötig.
**Hinweis:** Global `~/.claude` RTK/lean-ctx bleibt User-seitig optional zu bereinigen (B3, außer Repo).

## 2026-07-18 — Grok rules: `.grok/rules/00-dispatch.md`

**Begründung:** Tier B2 — Grok-spezifische Hinweise ohne CLAUDE.md aufzublähen; Plan-first für fail-closed/delivery-Pfade.
**Änderungen:**
- `.grok/rules/00-dispatch.md` (plan-first paths, tools, git, continuity)
**Ergebnis:** Doc-only.
**Hinweis:** Canonical remains `CLAUDE.md`; this file does not duplicate MUST invariants.

## 2026-07-18 — Release v0.6.0

**Begründung:** Erste GitHub-Version nach Code-Stand 0.5.0; Bündelung von P0-Auth, Worker-Reliability, Audit-Fixes und Tests.
**Änderungen:**
- `internal/version/version.go`, `Dockerfile`, `.github/workflows/build.yml` (`0.5.0` → `0.6.0`)
- GitHub Release `v0.6.0` mit Release Notes
**Ergebnis:** Version-Bump; Release über `gh release create`.

## 2026-08-18 — CI-Fix: Go 1.26.6 (govulncheck + Trivy stdlib-CVEs)

**Begründung:** Weekly Security-Run 31997859586 war rot: govulncheck und alle 4 Trivy-Jobs fanden HIGH-Stdlib-CVEs in go1.26.5 (u. a. CVE-2026-56862/56860/56853/33818, CVE-2026-39821/46600); alle in go1.26.6 behoben.
**Änderungen:**
- `go.mod` (toolchain go1.26.5 → go1.26.6)
- `.github/workflows/{build,security,integration}.yml` (setup-go 1.26.6)
- `Dockerfile` (`golang:1.26-alpine` Digest auf 1.26.6-Image), `docs/knowledge-base/overview.md`
**Ergebnis:** `devbox run lint` 0 Issues; `devbox run test` grün; `govulncheck ./...` → 0 affecting.
**Hinweis:** Security-Trivy scannt GHCR-`:latest` — nach Push auf main erst nach erfolgreichem `build.yml`-Image-Push wieder grün.

## 2026-08-21 — Worker Fetch(1), Quota CAS-Pause, KV-TTL reconcile, Sender-Cache 30s

**Begründung:** Code-Review gegen improvements.md: Fetch(10) startet AckWait ohne InProgress auf wartenden Msgs; Quota-CAS retried ohne Pause (503 vor 1MB); KV-TTL wurde nach Create ignoriert; 10-min Sender-Cache über Prozessgrenzen.
**Änderungen:**
- `internal/worker/consumer.go` (Fetch(1); 500ms Backoff bei Fetch-Fehler)
- `internal/quota/quota.go` (exponentielle Pause + Jitter zwischen CAS-Konflikten)
- `internal/natsutil/setup.go` (bestehende KV-Buckets: MaxAge/TTL reconcilen)
- `internal/sender/sender.go` (`DefaultCacheTTL` 10min → 30s); Tests + KB/ARCHITECTURE/README/`improvements.md` #16
**Ergebnis:** `devbox run test` grün; golangci auf geänderten Packages 0 Issues. Repo-weites `devbox run lint` rot an vorbestehendem SA5011 in `internal/loggy/loggy_test.go`.
**Hinweis:** **DESIGN-DECISION** Fetch(1) statt Worker-Concurrency/Ordering. KV-Watch (#16) zurückgestuft. Worker-Restart nötig für laufende Prozesse.

## 2026-08-21 — Docs-Audit + SA5011; Bounce/Object-Store Backlog

**Begründung:** Living Docs (Testzahlen 241/249, Coverage, Bounce-Gotcha, mutate-Liste) widersprachen dem Code; Bounce-KB behauptete „viele PATCHes“, der Client liest aber nur die erste Graph-Seite. Repo-Lint war rot an SA5011.
**Änderungen:**
- `internal/loggy/loggy_test.go` (unmöglichen nil-Check nach `GetLogger` entfernt)
- README/CLAUDE/ARCHITECTURE/KB (`decisions.md` Fetch(1)-ADR; Bounce-Gotcha; Coverage Stand 2026-08-21; Tests 251)
- `improvements.md` (#21 Graph nextLink, #22 Object-Store TTL; #12/#18 präzisiert)
**Ergebnis:** `devbox run lint` 0 Issues; `devbox run test` grün.
**Hinweis:** Historische 241/249 in älteren Log-Einträgen bleiben als Audit-Trail.

## 2026-08-21 — #12 Prometheus + traceContext (Logs/Audit/Graph)

**Begründung:** Keine Queue-/Latenz-Metriken; `traceContext` wurde nur im JSON durchgereicht, nicht in Logs/Audit/Graph. Fail-closed-Pfade unverändert.
**Änderungen:**
- `internal/metrics/` (Prometheus; `/metrics` Gateway/Admin; Worker/Bounce auf `PORT`)
- `internal/loggy` (`Context`/`WithContext`; ExternalApi* nimmt ctx); `domain.SanitizeTraceContext` (nur W3C)
- Gateway/Worker/MS Graph: sanitize, Audit-Feld, `client-request-id` + W3C-Header; Admin `MailRecord.traceContext`
- `improvements.md` #12 umgesetzt; README/ARCHITECTURE/KB
**Ergebnis:** `devbox run lint` 0 Issues; `devbox run test` → 265 PASS, 1 SKIP.
**Hinweis:** **DESIGN-DECISION** `/metrics` unauthenticated wie Health; `traceContext` Allowlist `traceparent`/`tracestate` (kein Header-Injection). Worker/Bounce binden `PORT` für Scrapes.

## 2026-08-21 — Produktions-Tests: Coverage, Mutation-Gate, Integration-CI

**Begründung:** Suite war für Fail-closed-Invarianten gut, als Release-Gate aber lückenhaft: `.gremlins.yaml` ignorierte Mutatoren, Quota-Exact-Limit/Cutoff ungetestet, `Consumer.Run` 0 %, Integration nicht auf PRs.
**Änderungen:**
- `.gremlins.yaml` (v0.6-Schema `mutants.<name>.enabled`; `invert-logical` an; Thresholds unter `unleash`)
- Quota/Worker/Gateway/Spam/natsutil-Tests (Exact-Limit, Embedded-NATS CAS, `Consumer.Run`, Upload, Auth-Kanten)
- `.github/workflows/integration.yml` (`pull_request`); Integration `Consumer.Run` nutzt jetzt wirklich `Run`
**Ergebnis:** `devbox run lint` 0 Issues; `devbox run test` → 321 PASS, 1 SKIP; `devbox run test-integration` grün.
**Hinweis:** Worker 94 %, Gateway 88 %, Quota 86 %, Spam 100 %. `devbox run mutate` ist wieder ein echtes Gate.

## 2026-08-21 — Release v0.7.0

**Begründung:** Bündelung seit v0.6.0: Fetch(1), Quota-CAS-Pause, KV-TTL, Sender-Cache 30s, #12 Prometheus+traceContext, Go 1.26.6, Produktions-Tests.
**Änderungen:**
- `internal/version/version.go`, `Dockerfile`, `.github/workflows/build.yml` (`0.6.0` → `0.7.0`)
- GitHub Release `v0.7.0` mit Release Notes
**Ergebnis:** Version-Bump; Release über `gh release create`.
**Hinweis:** Worker/Bounce binden `PORT` für `/metrics`+`/health` (Default 8080). Integration-CI: `TestIntegration_ConsumerRun_TestModeAcks` isolieren (nil attStore vs. parallele Gateway-IT).
