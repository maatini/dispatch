# Code & Architecture Audit — dispatch

> Principal-Engineer-Review des gesamten Projekts (~6k LOC Go).
> Verifiziert live: `go build` / `go vet` sauber, 151/151 Tests mit `-race` grün, `golangci-lint` 0 Issues.
> Kein Code wurde im Rahmen des Audits verändert.

## Executive Summary

**dispatch** ist ein gut durchdachtes Multi-Tenant-E-Mail-Delivery-System (REST → NATS JetStream → MS Graph) mit ungewöhnlich starker Dokumentation, disziplinierter Fehler-Taxonomie und einer CI-Pipeline, die die meisten Projekte dieser Größe übertrifft. Das Kerndesign — NATS als einziges State-Backend, fail-closed Quota, KV-basierte Idempotenz — ist solide. Das Audit fand jedoch **mehrere echte Korrektheitsbugs** (Circuit-Breaker-Fehler werden bei Graph-Ausfällen als permanent fehlklassifiziert, eine Dedup-Key-Kollision verwirkt reprozessierte Dead Letters still, die Bounce-Pipeline kann in Produktion nichts korrelieren, Spam-Dedup ist nicht atomar), dazu **null Integrationstests, obwohl CI-Job und README das Gegenteil behaupten**, und **keine Authentifizierung am Mail-Send-Endpunkt**.

**Gesamtbewertung: 7/10.** Exzellente Engineering-Hygiene und Architektur; Punkteabzug für funktionale Lücken im Bounce-Flow, untertestete NATS-nahe Komponenten (Admin-Resolver 13 %, Worker 55 %), reine Log-Observability und Reliability-Bugs, die genau unter Fehlerbedingungen zuschlagen.

## Key Strengths

- **Architektur-Klarheit**: 4 Services mit klarer Zuständigkeit, ein State-Backend (NATS), keine Polyglot-Persistenz. Die 7-Stage-Gateway-Pipeline und die Transient/Permanent-Fehlertrennung (`internal/msgraph/errors.go`) sind Lehrbuch-Design für Delivery-Systeme.
- **Failure-Mode-Denken**: Dedup-KV mit 7-Tage-TTL, Object-Store-Entkopplung der Anhänge mit passenden 72h-TTLs, fail-closed Quota mit CAS-Retries (`internal/quota/quota.go`). Die Resilience-Matrix im README ist ehrlich und größtenteils akkurat.
- **Verifizierte Quality Gates**: `go build`, `go vet`, `go test -race ./...` (151/151 — Badge akkurat), `golangci-lint run` (0 Issues). Mutation-Testing mit Efficacy-Schwellwerten (`.gremlins.yaml`) ist selten und lobenswert.
- **Security-Tooling**: CodeQL, govulncheck, Trivy + SARIF, SBOM-Generierung, digest-gepinnte Distroless-Images, gepinnte CI-Tool-Versionen, minimale Workflow-Permissions. Secrets verifiziert nicht in der Git-History (`git log -S "sqp_"` sauber; `.env`/`kladde.md` ungetrackt).
- **PII-Disziplin**: E-Mail-Maskierung (`internal/pii`) konsistent an allen Log-Grenzen.
- **Dokumentation**: README, ARCHITECTURE.md und `docs/knowledge-base/` sind akkurat zum Code — Pipeline-Stages, TTLs und Config-Vars wurden gegengeprüft.

## Critical Issues & Risks

1. **Offener Circuit Breaker wird als permanenter Fehler fehlklassifiziert → Mails fälschlich FAILED** (`internal/msgraph/client.go:101` + `internal/worker/processor.go:105`). Nach 5 konsekutiven transienten Fehlern öffnet der Gobreaker; `Execute` liefert dann `ErrTooManyRequests`, das kein `GraphTransientError` ist. Der Processor ACKt und schreibt FAILED. Während eines 30-Sekunden-Graph-Ausfalls stirbt jede verarbeitete Mail permanent, statt redeliveriert zu werden — exakt das Gegenteil des dokumentierten Resilience-Verhaltens.
2. **Dead-Letter-Reprocessing verwirkt still alle außer der ersten Nachricht** (`internal/admin/resolver.go:148` + `internal/worker/processor.go:53`). `ReprocessDeadLetter` republisht den Raw-Payload *ohne* `traceId`-Header; der Processor fällt auf `traceID = "unknown"` zurück und dedupliziert auf diesen Key. Jede weitere reprozessierte Nachricht innerhalb von 7 Tagen wird als "Duplikat" verschluckt. Batch-Reprocessing verliert Nachrichten ohne Fehlermeldung.
3. **Bounce-Korrelation kann in Produktion nicht funktionieren** (`internal/bounce/crawler.go:19` vs. `internal/msgraph/service.go:buildGraphEmail`). Der Crawler extrahiert `X-Dispatch-TraceId` per Regex aus NDR-Bodies, aber kein Code setzt diesen Header (`internetMessageHeaders` fehlt) auf ausgehenden Mails — `OriginalTraceID` ist immer leer. Schlimmer: `Run()` ruft `MarkAsRead` **auch dann auf, wenn das Bounce-Record-Publish fehlschlägt** — NDRs werden bei NATS-Fehlern konsumiert und verloren. `BouncedRecipient` wird nie befüllt; `BouncedAt` ist Crawl-Zeit, nicht Bounce-Zeit.
4. **Keine Authentifizierung am Send-Endpunkt** (`internal/gateway/handler.go:62`). `POST /dispatch/api/v1/mail/send` hängt nur hinter `middleware.Recoverer`. Jeder netzwerkerreichbare Aufrufer kann als beliebiger bekannter `appTag` senden (nur Domain-Whitelist und Quota begrenzen). Falls Netzwerk-Isolation intendiert ist, muss das als harte Deployment-Anforderung dokumentiert werden.
5. **Admin-Stream-Queries liefern still partielle Daten** (`internal/admin/resolver.go:179`). `readStream` nutzt `sub.NextMsg(0)`; verifiziert gegen nats.go v1.51.0-Source (`nats.go:5348-5412`): Timeout 0 läuft sofort ab, wenn der Buffer momentan leer ist → die Schleife bricht früh ab → nichtdeterministisch unvollständige Audit-/Bounce-/Dead-Letter-Ergebnisse, ohne Fehler. Zusätzlich wird der **gesamte 30-Tage-Stream pro Query in den Speicher gelesen** und `ctx` ignoriert.
6. **Spam-Dedup ist nicht atomar (TOCTOU)** (`internal/spam/spam.go:27`). Get-then-Put lässt zwei konkurrierende identische Requests beide passieren. Atomares KV `Create` ist der Ein-Zeilen-Fix.
7. **Null Integrationstests existieren** — `grep -r "go:build integration"` findet nichts, dennoch läuft wöchentlich ein CI-Job "Integration Tests" und das README referenziert Integration-only-Coverage. Der Job ist ein Placebo, das Unit-Tests erneut laufen lässt; alle NATS-nahen Pfade (Publisher, Consumer, Object Store, Admin-Resolver mit 13 % Coverage) sind Ende-zu-Ende ungetestet.
8. **Readiness-Probe lügt** (`internal/gateway/handler.go:175`). `/health` und `/health/ready` melden NATS immer "UP", ohne die Verbindung zu prüfen. Bei totem NATS bleibt das Gateway in Rotation und liefert 503s, statt ersetzt zu werden.

## Detailed Analysis

### Architecture & Design
Kohäsiv und gut geschichtet: `cmd` verdrahtet, `internal` enthält Logik, Interfaces sind an den Consumption-Points definiert (`handler.go:22-42`, `processor.go:23-38`). Bedenken: Jeder Service provisioniert Streams/KV bei jedem Start neu, und `upsertKV` reconciliert bestehende Bucket-Configs nie — eine Änderung von `DISPATCH_SPAM_TIMEOUT_SECONDS` bewirkt auf einem existierenden Bucket still nichts (`natsutil/setup.go:120`). Subject-Namen (`cody.mailing.*`) sind Legacy-inkonsistent zur `DISPATCH_*`-Benennung. Keine Deployment-Manifeste im Repo — Deployment-Topologie ist nicht bewertbarer Kontext.

### Code Quality & Maintainability
Hoch: kleine Funktionen, konsistente Namen, `%w`-Wrapping, typisierte Domain-Errors. Verstöße: `validateRequest` liefert `ErrUnknownAppTag` für **jeden** Struct-Validierungsfehler (`validation.go:20`) — ein malformed Recipient wird als unbekannter appTag gemeldet; `writeValidationError`-Fallthrough emittiert HTTP 500 mit leerem `code` (`handler.go:188`), was den API-Vertrag bricht; Base64-Anhänge werden zweimal vollständig dekodiert (Validierung + Upload); `config.envInt` verschluckt Parse-Fehler still — Tippfehler in numerischen Env-Vars sind unsichtbar.

### Performance & Scalability
Der Worker verarbeitet Nachrichten strikt sequenziell (`consumer.go:49`) mit `AckWait: 30s`, während eine Nachricht legitim länger brauchen kann (Rate-Limiter-Wait + bis zu 2×30s Retry-Sleeps + 16 Chunks bei 20MB-Anhang); es gibt kein `InProgress()`-Deadline-Extension, also redeliverieren In-Flight-Messages mitten in der Verarbeitung — das Dedup-KV ist die einzige Doppelsendungs-Bremse, und sein `Put`-Fehler ist Warn-and-Continue. Throughput-Decke ~1 Msg/s/Sender. Der Quota-KV-Value wächst um einen JSON-Eintrag pro Request und wird jedes Mal komplett neu geschrieben — ab ~30k Requests/Tag/appTag nähert er sich NATS' 1MB Max-Payload, danach schlägt Quota fail-closed → 503-Ausfall für genau den größten Tenant. `RateLimiter.limiters` wächst unbeschränkt (minor). Admin-Resolver sind O(gesamter Stream) pro Query (Critical #5).

### Testing Strategy & Quality
Unit-Tests sind genuin gut: table-driven, race-sauber, Stub-basiert, mit echten Verhaltensassertionen (z. B. `processor_test.go` assertiert: transienter Fehler → kein Audit). 100 % Mutation-Efficacy auf 7 Packages untermauert das. Lücken: keine Integrationstests (#7); `internal/admin` bei 13 % — der fehleranfälligste Code ist am wenigsten getestet; `internal/worker` 55 % — Tests bauen `Processor` per Struct-Literal mit nil `attStore`, eine latente Panik bei jeder Nachricht mit Anhängen; kein Test für fehlenden `exp`-JWT-Claim.

### Error Handling, Logging & Observability
Die Transient/Permanent-Taxonomie ist exzellent, hat aber drei Klassifikationsbugs: Breaker-open (#1); Token-Endpoint-Fehler, bei denen **jeder** non-200 — inklusive 400 invalid-credentials — transient wird → Infinite-Redelivery bei Konfigurationsfehler (`token.go:95`); und Chunk-Uploads, bei denen jeder Status ≥400 — inklusive permanenter 4xx — transient wird (`service.go:uploadChunks`). Logging ist strukturiert, kategorisiert, PII-maskiert — aber es gibt **keine Metriken und keine Traces**; `traceContext` wird von der API akzeptiert und nie in Logs, Graph-Calls oder Audit-Records propagiert. Queue-Depth, Graph-Latenz und per-Tenant-Delivery-Status sind ohne Log-Stöbern nicht beantwortbar. `token.go` nutzt `http.DefaultClient` ohne Timeout (Hang-Risiko); `uploadChunks` umgeht Breaker und Retry komplett.

### Dependency Management & Tech Stack
Schlank und aktuell (Go 1.25.9, 8 direkte Deps), keine roten Flaggen. `gobreaker` v1.0.0 ist stabil, aber faktisch unmaintained; `graph-gophers/graphql-go` ist Maintenance-Mode — jetzt ok, langfristig beobachten. CI pint Versionen; das Dockerfile ist exemplarisch (distroless, nonroot, digest-gepinnt, trimpath).

### Documentation & Developer Experience
Best-in-Class für diese Größe: devbox-Reproduzierbarkeit, Dev-Proxy-Mocks ohne Azure, Bruno-Collection, akkurate Resilience-Matrix. Abzüge: README behauptet "exp-Claim erforderlich", aber `auth.go` erzwingt ihn nicht (jwt/v5 validiert `exp` nur wenn vorhanden); Integrationstest-Skript/CI-Job sind irreführend (#7); `kladde.md` (gitignored) enthält einen live Sonar-Token im Klartext — rotieren; `tools/gen-admin-token` fällt still auf `"dev-secret"` zurück.

### Bugs & Edge Cases (jenseits der Criticals)
Spam-KV-Fehler mappen auf 500-mit-leerem-Code, Quota-Fehler auf 503 — inkonsistenter Failure-Vertrag für dieselbe Fehlerklasse. `handler.go:113` und `validation.go:81` nutzen `append(append(req.Recipients, cc...), bcc...)`, was das Recipients-Backing-Array aliasen kann (heute harmlos, fragil). `expiresIn-60` im Token-Cache wird negativ, falls Graph je <60s liefert. `MaxDeliver: -1` ohne Backoff = Poison-Messages redeliverieren ewig; nur JSON-Parse-Fehler erreichen den DLQ. Bei permanentem NATS-Verlust dreht der Consumer-Fetch-Loop ewig Warnungen, statt für Orchestrator-Restart zu crashen. Audit-/Dead-Letter-Publish-Fehler sind log-only, während die Nachricht ACKt wird — stiller Audit-Verlust.

### Modernization Opportunities
Go-1.25-Features sind im Einsatz. Reale Chancen: KV `Create` für Spam; KV-Watcher für Sender-Cache-Invalidierung (heute: 10-min Cross-Replica-Staleness); OTel-SDK für den Trace-Kontext, den die API bereits akzeptiert; `MsgMetadata()`-Delivery-Counts für DLQ-Eskalation; eine echte slog-Handler-Config statt loggys per-Instanz-Handler-Bau.

## Prioritized Improvement Opportunities

### Quick Wins (High Impact, Low-Medium Effort)

| # | Änderung | Impact | Effort | Risiko | Warum es zählt |
|---|----------|--------|--------|--------|----------------|
| 1 | `gobreaker.ErrTooManyRequests` als `GraphTransientError` wrappen in `client.do` | High | Low | Low | Fixt falsche FAILED bei Graph-Ausfällen (#1) |
| 2 | `X-Dispatch-TraceId` via `internetMessageHeaders` in `buildGraphEmail` setzen; `MarkAsRead` nur nach erfolgreichem Publish; Recipient/receivedDateTime extrahieren | High | Low | Low | Macht das gesamte Bounce-Feature funktional; stoppt NDR-Verlust (#3) |
| 3 | Dead Letters mit originalem `traceId`-Header republishen (aus Payload parsen) | High | Low | Low | Fixt stille Reprocessing-Verluste (#2) |
| 4 | `spam.Check`: atomares KV `Create` statt Get+Put | High | Low | Low | Schließt TOCTOU-Race (#6) |
| 5 | Echte Readiness: NATS-Konnektivität in `/health/ready` prüfen | High | Low | Low | Kaputte Pods verlassen die Rotation (#8) |
| 6 | `readStream`: `NextMsg(5*time.Second)` + `ctx` respektieren | High | Low | Low | Korrekte, vollständige Admin-Ergebnisse (#5) |
| 7 | Validierungs-Error-Codes korrigieren; nie leeren `code` emittieren; Spam-/Quota-Fehler konsistent auf 503 mappen | Medium | Low | Low | API-Vertrags-Ehrlichkeit |
| 8 | Token-Client: dedizierter `http.Client` mit Timeout; Token-4xx als permanent klassifizieren | Medium | Low | Low | Verhindert Hangs und Infinite-Redelivery bei falschen Credentials |
| 9 | `exp`-Claim erzwingen (`WithExpirationRequired`) oder README korrigieren | Medium | Low | Low | Doku/Code-Security-Mismatch |
| 10 | Sonar-Token in `kladde.md` rotieren | Medium | Low | Low | Klartext-Secret auf Disk |

### Medium-term Improvements

| # | Änderung | Impact | Effort | Risiko | Warum es zählt |
|---|----------|--------|--------|--------|----------------|
| 11 | Echte Integrationstests (testcontainers/NATS) für Publisher, Consumer, Object Store, Admin-Resolver; CI-Job ehrlich machen | High | Medium | Low | Der riskanteste Code (13–55 % Cov) ist NATS-nah |
| 12 | Prometheus-Metriken + `traceContext` in Logs/Audit/Graph propagieren | High | Medium | Medium | Null Observability in einer Delivery-Pipeline |
| 13 | Worker-Ack-Management: `InProgress()`-Heartbeats oder größeres `AckWait`; Delivery-Count-basierte DLQ-Eskalation statt `MaxDeliver: -1` | High | Medium | Medium | Eliminiert Redelivery-Races und Poison-Message-Loops |
| 14 | AuthN/AuthZ-Entscheidung für `/mail/send` (mTLS, Token oder dokumentierte Isolation); per-Tenant-Admin-Authorisierung | High | Medium | Medium | Schließt die Spoofing-Frage (#4) |
| 15 | Quota-Store: Einträge aggregieren (Per-Minute-Buckets) statt Per-Request-Einträge | Medium | Medium | Medium | Verhindert 1MB-Value-Fail-Closed-Ausfall für Top-Tenants |
| 16 | Sender-Cache-Invalidierung via KV Watch; KV-Config-Drift reconcilieren (oder dokumentieren) | Medium | Medium | Medium | Entfernt 10-min Cross-Replica-Staleness |

### Strategic / Larger Refactors

| # | Änderung | Impact | Effort | Risiko | Warum es zählt |
|---|----------|--------|--------|--------|----------------|
| 17 | Admin-Queries: indexierte Speicherung oder Consumer-basiertes Paging statt Full-Stream-Scans | High | High | Medium | 30-Tage-Streams überleben In-Memory-Pagination nicht |
| 18 | Worker-Concurrency: begrenzte Parallelverarbeitung mit Per-Sender-Ordering | Medium | High | High | Entfernt die ~1 Msg/s/Sender-Decke |
| 19 | Deployment-Manifeste (Kustomize/Helm) + Version aus Git-Tag statt hardcodiertem `VERSION=0.5.0` | Medium | Medium | Low | Deploybarkeit ist derzeit Tribal Knowledge |
| 20 | `graph-gophers/graphql-go` neu bewerten, bevor die Admin-API wächst | Low | High | Medium | Vermeidet spätere Zwangsmigration |

## Recommended Action Plan

**Phase 1 — Korrektheit unter Fehlerbedingungen.** Items 1–3, 5–8, 10: kleine chirurgische Diffs an identifizierten Zeilen, jeweils mit Regressions-Unit-Test (Stub-Infrastruktur existiert; ein Breaker-open-Test ist trivial via 5 transienter Fehler aus `stubGraph`). Zusammen shippen.

**Phase 2 — Ehrliche Verifikation.** Item 11: testcontainers-basierte Integrationstests für Publish→Consume→Send, Attachment-Roundtrip und Admin-Resolver-Reads; Placebo-CI-Job fixen oder löschen. Worker-Attachment-Pfad-Unit-Tests ergänzen (nil-`attStore` ist eine latente Panik). Item 9 parallel.

**Phase 3 — Operability.** Items 12–14: Metriken pro Service (Queue-Depth, Graph-Latenz, Quota-Headroom, Dedup-Hits), TraceContext-Propagation, Ack-Deadline-Handling und eine dokumentierte, durchgesetzte Entscheidung zur Send-Endpunkt-Authentifizierung. Item 15, sobald ein Tenant ~10k Mails/Tag erreicht.

**Phase 4 — Scale-out.** Items 17–19, wenn Audit-Volumen oder Tenant-Anzahl sie schmerzhaft machen; Item 20 nur wenn die Admin-API wachsen muss.

**Höchste Hebelwirkung zuerst (Top 5):** (1) Breaker-Fehlklassifizierung, (2) Bounce-Trace-ID-Header + Mark-as-Read-Reihenfolge, (3) Dead-Letter-Reprocess-Header, (4) Spam atomares Create, (5) Integrationstests, die tatsächlich existieren. Die ersten vier sind je unter ~20 geänderte Zeilen und fixen Verhalten, das Kundenmails still verliert oder fälschlich failt; der fünfte beweist es und hält es gefixt.

---

**Verifikations-Evidenz:** Nur Analyse — keine Dateien modifiziert (`git status` sauber). Live ausgeführte Kommandos: `go build ./...` (sauber), `go vet ./...` (sauber), `go test -race ./...` (151 pass / 0 fail), `golangci-lint run ./...` (0 Issues), `grep -r "go:build integration"` (keine Integrationstests existieren), `git log --all -S "sqp_"` (kein Secret in History), NextMsg(0)-Semantik verifiziert gegen nats.go v1.51.0 Modul-Source.
