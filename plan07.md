# Umsetzungsplan — dispatch Audit-Fixes (Phase 1 + 2)

> **Zielgruppe dieses Plans:** DeepSeek V4 Pro (ausführendes Coding-Modell)
> **Quelle:** `improvements.md` (vollständiges Audit). Dieser Plan operationalisiert die Quick Wins (#1–#10) und die Test-Verifikation (#11, teilw. #9).
> **Modus:** Implementierung. Jede Aufgabe ist eigenständig verifizierbar und committbar.

---

## 0. Kontext für das ausführende Modell

**Projekt:** `dispatch` — Multi-Tenant-E-Mail-Delivery-System in Go 1.25.
Vier Services unter `cmd/` (mail-gateway, mail-worker, mail-admin, bouncemanagement), Fachlogik unter `internal/`. State-Backend ist ausschließlich NATS JetStream (Streams, KV, Object Store). Versand via Microsoft Graph API.

**Verifizierter Ausgangszustand (nicht erneut auditieren):**
- `go build ./...`, `go vet ./...` — sauber
- `go test -race ./...` — 151 Tests grün
- `golangci-lint run ./...` — 0 Issues

**Globale Regeln (verbindlich):**
1. **Nur die spezifizierten Änderungen.** Keine Refactorings, keine Kommentar-/Formatänderungen an unbeteiligtem Code, keine neuen Dependencies außer wo explizit genannt.
2. **Jede Aufgabe endet mit:** `go build ./... && go vet ./... && go test -race ./... && golangci-lint run ./...` — alle vier müssen grün sein, bevor die nächste Aufgabe beginnt.
3. **Pro Aufgabe mindestens ein Regressions-Test**, der ohne den Fix rot wäre. Test-Stil: bestehende Stub-Muster verwenden (siehe `internal/worker/processor_test.go`, `internal/gateway/handler_test.go`).
4. **Bestehende Konventionen halten:** Errors mit `%w` wrappen, Domain-Error-Typen aus `internal/domain/errors.go` nutzen, PII-Maskierung (`pii.MaskEmail`) an Log-Grenzen, Interfaces am Consumption-Point definieren.
5. **Keine neuen Kommentare** außer wo die Logik nicht selbsterklärend ist (Projektkonvention).
6. Commit-Granularität: ein Commit pro Aufgabe, Message-Schema `fix(<package>): <kurz>` bzw. `test(<package>): <kurz>`.

**Reihenfolge:** Aufgaben 1→10 strikt sequenziell. Abhängigkeiten sind wo vorhanden vermerkt.

---

## Aufgabe 1 — Circuit-Breaker-Fehler als transient klassifizieren

**Problem:** Öffnet der Gobreaker (5 konsekutive transiente Fehler), liefert `breaker.Execute` `gobreaker.ErrTooManyRequests`. Das ist kein `*GraphTransientError`, daher behandelt `doWithRetry` es als permanent und der Worker ACKt Mails mit FAILED — während eines Graph-Ausfalls genau das falsche Verhalten.

**Datei:** `internal/msgraph/client.go`

**Änderung:**
1. Import `"github.com/sony/gobreaker"` ist bereits vorhanden.
2. In `func (c *Client) do(...)`, im Block `if cbErr != nil { ... }`: vor dem `return` prüfen, ob `errors.Is(cbErr, gobreaker.ErrTooManyRequests)` (bzw. `gobreaker.ErrOpenState` — je nachdem, was die installierte Version v1.0.0 exportiert; mit `go doc github.com/sony/gobreaker` verifizieren). In diesem Fall `cbErr` in `&GraphTransientError{Cause: cbErr}` wrappen.

**Regressions-Test:** `internal/msgraph/client_test.go` (Existenz prüfen, sonst neu anlegen): Test `TestDo_BreakerOpenIsTransient` — Client mit Breaker-Settings, die nach 1 Fehler öffnen (`ReadyToTrip: counts.ConsecutiveFailures >= 1`), httptest-Server der zweimal 500 liefert. Erster Aufruf → transient; zweiter Aufruf (Breaker offen) → Fehler muss per `errors.As(err, &transient)` als `*GraphTransientError` erkennbar sein. Ohne Fix: rot.

**Akzeptanzkriterien:** Neuer Test grün; bestehender Testbestand grün; Worker-seitig führt ein offener Breaker damit zu Nicht-ACK (Redelivery), nicht zu FAILED-Audit.

---

## Aufgabe 2 — Token-Fetch: Timeout + 4xx als permanent

**Problem:** `fetchToken` nutzt `http.DefaultClient` (kein Timeout → Hang-Risiko) und klassifiziert **jede** non-200-Antwort (inkl. 400/401 bei falschen Credentials) als `GraphTransientError` → Infinite-Redelivery bei Konfigurationsfehlern.

**Datei:** `internal/msgraph/token.go`

**Änderung:**
1. Package-Level: `var tokenHTTPClient = &http.Client{Timeout: 15 * time.Second}`; in `fetchToken` `http.DefaultClient.Do(req)` durch `tokenHTTPClient.Do(req)` ersetzen.
2. Statuscode-Behandlung: bei `resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests` → `&GraphPermanentError{StatusCode: resp.StatusCode, Body: string(body)}` zurückgeben. Bei 429 und ≥500 weiterhin `GraphTransientError`.

**Regressions-Test:** `internal/msgraph/token_test.go` erweitern: `TestFetchToken_ClientErrorIsPermanent` — httptest-Server liefert 400 mit Body `{"error":"invalid_client"}`; Assertion: `errors.As(err, &perm)` mit `*GraphPermanentError` trifft zu. Zweiter Test `TestFetchToken_ServerErrorIsTransient` mit 503 → `*GraphTransientError`. (Falls `tokenCache.tokenEndpointBase` für die Testadresse gesetzt werden muss: vorhandenes Test-Muster aus `token_test.go` nutzen.)

**Akzeptanzkriterien:** 400/401 aus dem Token-Endpoint führt im Worker zum FAILED-Audit (sichtbarer, sofortiger Fehler statt Endlosschleife); 429/5xx bleibt transient.

---

## Aufgabe 3 — Bounce-Pipeline funktionsfähig machen

**Problem (dreiteilig):**
a) Ausgehende Mails tragen keinen `X-Dispatch-TraceId`-Header → der Crawler-Regex (`internal/bounce/crawler.go:19`) kann nie matchen.
b) `Crawler.Run` ruft `MarkAsRead` auch nach fehlgeschlagenem `process()` auf → NDR-Verlust bei NATS-Fehlern.
c) `BounceRecord.BouncedRecipient` bleibt immer leer.

**Dateien:** `internal/msgraph/service.go`, `internal/bounce/crawler.go`, ggf. `internal/msgraph/bounce.go`

**Änderung a) — Header setzen:**
In `buildGraphEmail` (`service.go`): `graphMessage` um Feld `InternetMessageHeaders []graphMessageHeader `json:"internetMessageHeaders,omitempty"`` erweitern; Typ `graphMessageHeader{Name string `json:"name"`; Value string `json:"value"`}`. Immer einen Header `{Name: "X-Dispatch-TraceId", Value: req.TraceID}` setzen. (Graph API v1.0 unterstützt `internetMessageHeaders` auf der Message-Resource; gilt für sendMail und Draft gleichermaßen, da `buildGraphEmail` für beide Pfade genutzt wird.)

**Änderung b) — MarkAsRead nur bei Erfolg:**
In `crawler.go`, Methode `Run`: den `MarkAsRead`-Aufruf in den Erfolgszweig verschieben — nur aufrufen, wenn `c.process(ctx, msg)` `nil` zurückgab. Bei Fehler: warn loggen (bestehend), `MarkAsRead` auslassen (NDR bleibt ungelesen → nächster Crawl-Versuch).

**Änderung c) — Recipient extrahieren:**
`GetUnreadMessages` (`internal/msgraph/bounce.go`): `$select=id,subject,body` erweitern um `,toRecipients,receivedDateTime`; Response-Struct entsprechend ergänzen (`ToRecipients []struct{EmailAddress struct{Address string `json:"address"`}}`, `ReceivedDateTime time.Time`). `NDRMessage` um `Recipient string` und `ReceivedAt time.Time` erweitern und befüllen (erster To-Recipient, falls vorhanden). Im Crawler `process()`: `BouncedRecipient: msg.Recipient` setzen; `BouncedAt`: `msg.ReceivedAt` verwenden, Fallback `time.Now().UTC()` wenn zero.

**Regressions-Tests:**
- `internal/msgraph/service_test.go`: `TestBuildGraphEmail_SetsTraceHeader` — Payload enthält `internetMessageHeaders` mit Name `X-Dispatch-TraceId` und Value = TraceID.
- `internal/bounce/crawler_test.go`: Test mit Stub-Graph, dessen `MarkAsRead` einen Zähler führt, und Stub-JS, dessen `Publish` beim 2. Aufruf fehlschlägt → Assertion: `MarkAsRead` wurde für die fehlgeschlagene Nachricht **nicht** aufgerufen, für erfolgreiche schon. Bestehender Stub-Stil in `crawler_test.go` wiederverwenden.

**Akzeptanzkriterien:** Neu verschickte Mails tragen den Trace-Header; bei NATS-Ausfall bleiben NDRs ungelesen und werden erneut verarbeitet; Bounce-Records enthalten Recipient und echte Bounce-Zeit.

---

## Aufgabe 4 — Dead-Letter-Reprocessing: TraceID-Header wiederherstellen

**Problem:** `ReprocessDeadLetter` publisht den rohen Payload ohne Header. Der Worker-Dedup-Key kommt aus dem Header und fällt auf `"unknown"` zurück → nach der ersten reprozessierten Nachricht werden alle weiteren innerhalb von 7 Tagen als Duplikate verworfen.

**Datei:** `internal/admin/resolver.go`

**Änderung:** In `ReprocessDeadLetter`: Payload in `domain.MailRequestDO` parsen (`json.Unmarshal`). Schlägt das fehl → GraphQL-Fehler `"invalid dead letter payload"` zurückgeben (fail fast statt Blind-Publish). Bei Erfolg: `nats.NewMsg(natsutil.SubjectMails)` bauen, `Header.Set("traceId", req.TraceID)` und `Header.Set("appTag", req.AppTag)` setzen (Spiegelung des Gateway-Verhaltens in `internal/gateway/publisher.go`), `Data = []byte(args.Payload)`, via `r.js.PublishMsg(msg)` publishen. Schlägt der Unmarshal fehl, weil das Dead-Letter-Payload selbst korrupt ist (Hauptgrund für Dead Letters!), **dann** bewusst als Raw-Payload mit frischer UUID (`google/uuid`, bereits Dependency) als traceId publishen — dokumentiert durch Test.

**Regressions-Test:** `internal/admin` — Resolver mit Fake-`js` (Interface-Extraktion nötig? `Resolver.js` ist `nats.JetStreamContext` — für den Test eine minimale Interface `mailPublisher{PublishMsg(...)}` einführen oder bestehendes Test-Setup prüfen): Zwei Aufrufe von `ReprocessDeadLetter` mit zwei verschiedenen gültigen Payloads unterschiedlicher TraceIDs → Assertion: beide publishen mit jeweils korrektem `traceId`-Header. (Zusätzlich dokumentiert dieser Test Aufgabe-4-Fix gegen die "unknown"-Kollision; die Kollisionsvermeidung selbst verifiziert Aufgabe 5.)

**Akzeptanzkriterien:** Mehrere reprozessierte Dead Letters mit unterschiedlichen TraceIDs werden alle zugestellt (keine Dedup-Kollision mehr).

---

## Aufgabe 5 — Worker-Dedup: TraceID aus Payload, nicht aus Header-Fallback

**Problem (härtet Aufgabe 4 ab):** `Processor.Handle` nutzt `msg.Header.Get("traceId")` mit Fallback `"unknown"` für Dedup und Audit-Korrelation. Header und Payload können auseinanderlaufen; der "unknown"-Key ist eine globale Sammelstelle.

**Datei:** `internal/worker/processor.go`

**Änderung:** Nach erfolgreichem `json.Unmarshal(msg.Data, &req)`: wenn `req.TraceID` nicht-leer ist, diese als Dedup-Key und für Logs verwenden (Header-Wert nur als Fallback, wenn `req.TraceID == ""`). Ist **beides** leer → Dead-Letter schreiben (Grund: `missing traceId`) und ACK — keine Dedup unter `"unknown"` mehr. Den bisherigen `"unknown"`-Pfad entfernen.

**Regressions-Test:** `internal/worker/processor_test.go`: `TestHandle_MissingTraceID_GoesToDeadLetter` — Message ohne Header und mit Payload ohne TraceID → Assertion: Dead-Letter-Stream-Publish erfolgt, `delivered`-KV bleibt leer, kein SendEmail-Aufruf. Zweiter Test `TestHandle_DedupUsesPayloadTraceID` — Header-TraceID ≠ Payload-TraceID; Dedup-Eintrag muss unter der Payload-TraceID landen.

**Akzeptanzkriterien:** Zwei header-lose Nachrichten mit verschiedenen Payload-TraceIDs kollidieren nicht; header+payload-lose Nachrichten gehen in den DLQ statt in die Sammel-"unknown"-Dedup.

---

## Aufgabe 6 — Spam-Dedup atomar (TOCTOU)

**Problem:** `spam.Check` macht Get-then-Put — zwei konkurrierende identische Requests passieren beide (Race am Gateway).

**Datei:** `internal/spam/spam.go`

**Änderung:** `kvStore`-Interface um `Create(key string, value []byte) (uint64, error)` erweitern (nats.KeyValue erfüllt das bereits). `Check` umbauen: direkt `c.kv.Create(hash, []byte{1})` aufrufen. Bei Erfolg → `nil` (Hash neu). Bei Fehler: `errors.Is(err, nats.ErrKeyExists)` → `&domain.ValidationError{Code: domain.ErrSpamDetected, ...}`; sonstiger Fehler → gewrapped error (fail-closed-Verhalten wie bisher beibehalten). Den Get-Pfad entfernen.

**Regressions-Test:** `internal/spam/spam_test.go`: Stub-KV mit `Create`, das bei existierendem Key `nats.ErrKeyExists` liefert (Stub-Pattern aus `quota_test.go` übernehmen — dort gibt es CAS-Stubs). Tests: erster Check → nil; zweiter Check gleicher Hash → `*domain.ValidationError` mit Code `SPAM_DETECTED`; KV-Fehler → generischer Error (kein ValidationError).

**Akzeptanzkriterien:** Unter Nebenläufigkeit (zwei parallele Gateway-Requests, gleicher Hash) kann nur einer passieren — KV-seitig erzwungen, nicht applikationsseitig geraten.

---

## Aufgabe 7 — Echte Readiness-Probe

**Problem:** `/health` und `/health/ready` melden NATS immer "UP" ohne Verbindungsprüfung.

**Dateien:** `internal/gateway/handler.go`, `cmd/mail-gateway/main.go`

**Änderung:**
1. `Handler` um Feld `natsStatus func() nats.Status` (oder minimaler Interface `statusChecker{ Status() nats.Status }`) erweitern; in `main.go` beim `NewHandler`-Aufruf `nc.Status` (Method Value der bestehenden `*nats.Conn`) übergeben — Signatur von `NewHandler` entsprechend erweitern und alle Aufrufstellen (inkl. Tests) anpassen.
2. `handleHealth`: `status := "UP"`, HTTP 200; wenn `natsStatus() != nats.CONNECTED` → `"DOWN"`, HTTP 503, Check-Eintrag `{"name":"nats","status":"DOWN"}`. `/health/live` bleibt unverändert immer 200 (Liveness = Prozess lebt). `/health/ready` nutzt dieselbe Logik wie `/health`.

**Regressions-Test:** `internal/gateway/handler_test.go`: `TestReady_NatsDown_Returns503` — Handler mit Stub `func() nats.Status { return nats.DISCONNECTED }` → GET `/health/ready` → Status 503, Body enthält `"DOWN"`. Plus Erfolgsfall mit `nats.CONNECTED` → 200.

**Akzeptanzkriterien:** Bei NATS-Verlust meldet Readiness DOWN/503; Liveness bleibt 200.

---

## Aufgabe 8 — Admin-Stream-Reads: Timeout + Kontext

**Problem:** `readStream` nutzt `sub.NextMsg(0)` — Timeout 0 läuft sofort ab (verifiziert in nats.go v1.51.0) → nichtdeterministisch partielle Ergebnisse. `ctx` wird ignoriert.

**Datei:** `internal/admin/resolver.go`

**Änderung:**
1. `readStream`: `sub.NextMsg(0)` → `sub.NextMsg(5 * time.Second)`; Schleifenabbruch bei `nats.ErrTimeout` (alle verfügbaren Messages gelesen) — weiterhin früher Ausstieg bei `len(results) >= info.State.Msgs`, aber **vor** dem Unmarshal-Filter zählen: separaten Zähler `consumed` für jede empfangene Message führen (unabhängig vom Unmarshal-Erfolg), damit korrupte Messages den Abbruch nicht verhindern.
2. `ctx` durchreichen: `readAuditStream(ctx)` etc. geben ctx an `readStream`; vor jedem `NextMsg` `ctx.Err()` prüfen und ggf. abbrechen (`return nil, ctx.Err()`).
3. Kein Umbau der Gesamtarchitektur (Full-Stream-Read) — das ist Item #17 aus dem Audit und explizit nicht Teil dieses Plans.

**Regressions-Test:** Sofern NATS-fakebar (bestehende Admin-Tests prüfen): Test mit simuliertem Stream, der langsamer liefert als der Konsum → vollständige Ergebnismenge statt Frühabbruch. Falls ohne NATS nicht testbar: Testlücke als Kommentar im PR benennen und durch Aufgabe 10 (Integrationstests) abdecken lassen.

**Akzeptanzkriterien:** Admin-Queries liefern die vollständige Message-Menge deterministisch; abgebrochene Requests terminieren die Schleife.

---

## Aufgabe 9 — API-Fehlerkontrakt: korrekte Codes, konsistente 503

**Problem (dreiteilig):**
a) `validateRequest` meldet jeden Struct-Validierungsfehler als `UNKNOWN_APP_TAG` (`internal/gateway/validation.go:20`).
b) `writeValidationError`-Fallthrough emittiert HTTP 500 mit leerem `code` (`handler.go`).
c) Spam-KV-Fehler landen als 500 (Pfad a/b), Quota-KV-Fehler als 503 — inkonsistent für dieselbe Fehlerklasse.

**Dateien:** `internal/gateway/validation.go`, `internal/gateway/handler.go`, `internal/domain/errors.go`, `internal/spam/spam.go`

**Änderung:**
1. `domain/errors.go`: neue Codes `ErrValidationFailed ErrorCode = "VALIDATION_FAILED"` und `ErrInternal ErrorCode = "INTERNAL_ERROR"` ergänzen.
2. `validation.go`: Struct-Validierungsfehler → `Code: domain.ErrValidationFailed` (Message unverändert).
3. `spam.go`: KV-Fehler in einen neuen Domain-Typ `&domain.SpamStateError{Cause: err}` (analog `QuotaStateError`, mit `Unwrap`) wrappen — Datei `internal/domain/errors.go` ergänzen.
4. `handler.go`: `writeValidationError`-Fallthrough: `writeError(w, http.StatusInternalServerError, domain.ErrInternal, "internal error", traceID)` (nie leerer Code, keine internen Fehlertexte nach außen). In `handleSend`, Stage-5-Fehlerbehandlung: `errors.As` auf `*domain.SpamStateError` → 503 analog Quota-Pfad; `*domain.ValidationError` → 400 wie bisher.

**Regressions-Test:** `internal/gateway/handler_test.go` bzw. `validation_test.go`: Request mit ungültiger Empfänger-Adresse (aber gesetztem appTag) → Response-Code `VALIDATION_FAILED`, nicht `UNKNOWN_APP_TAG`. Spam-State-Fehler (Stub) → 503. Interner Nicht-Domain-Fehler → 500 mit `INTERNAL_ERROR`.

**Akzeptanzkriterien:** Kein Response-Body mit leerem `code` mehr möglich; Fehlercodes semantisch korrekt; Spam/Quota-State-Fehler beide 503.

---

## Aufgabe 10 — Integrationstests real machen (Minimal-Suite)

**Problem:** Kein einziges `//go:build integration`-File existiert; der CI-Job und das devbox-Skript laufen ins Leere.

**Neue Dateien (alle mit Build-Tag `//go:build integration`):**
1. `internal/gateway/integration_test.go` — End-to-End gegen reales NATS (URL aus `NATS_URL`, Default `nats://localhost:4222`; Skip via `t.Skip` wenn nicht erreichbar — kurzer Dial-Check):
   - Setup: verbinden, Streams/KV/ObjectStore provisionieren (`natsutil`-Funktionen direkt nutzen), Sender in `senders`-KV anlegen.
   - Test A (Happy Path): HTTP POST gegen `handler.Router()` → 202; danach Message im Stream `DISPATCH_MAILS` vorhanden (Pull-Fetch, 5s Timeout) mit korrekter `traceId`-Header/Payload-Konsistenz; Attachment-Object im Object Store auffindbar.
   - Test B (Quota fail-closed): Sender mit `dailyQuota: 1`, zwei Requests mit je 1 Empfänger → zweiter Request 429 mit `X-RateLimit-*`-Headern.
2. `internal/worker/integration_test.go`:
   - Test C (Attachment-Roundtrip): AttachmentDO ins Object Store legen → `AttachmentStore.Fetch` liefert Bytes identisch zurück → `Cleanup` entfernt das Objekt (Get danach → Fehler).
   - Test D (Transient → Nicht-ACK → Redelivery): Payload publishen, Processor mit Graph-Stub (transienter Fehler, über `NewProcessor` mit Interface-Kompatibilität — Signatur prüfen, ggf. Test-Hilfskonstruktor) → Consumer einmal fetchen lassen → Message wird erneut zugestellt (Redelivery-Count steigt) statt im DLQ/Audit zu landen.
3. `internal/admin/integration_test.go`:
   - Test E: Audit-Records in `DISPATCH_AUDIT` publishen (inkl. eines absichtlich korrupten Records) → `Resolver.Mails` liefert alle wohlgeformten Records vollständig (deckt Aufgabe 8 ab).

**Verifikation lokal:** `docker compose up -d nats && go test -tags integration -race -timeout 120s ./...` — muss A–E grün ausführen.

**CI:** `.github/workflows/integration.yml` braucht **keine** Änderung (Job läuft bereits mit NATS-Service + `-tags integration`) — nach diesem Task wird er erstmals echte Integrationstests ausführen. In `README.md` den Satz zu Integrationstests unverändert lassen (er stimmt dann).

**Akzeptanzkriterien:** `go test -tags integration ./...` führt ≥5 neue Tests aus; ohne laufendes NATS skippen sie sauber (kein Fail auf Entwicklermaschinen ohne Docker); `-race` sauber.

---

## Abschluss-Protokoll (nach Aufgabe 10)

1. Vollständige Suite: `go build ./... && go vet ./... && go test -race ./... && golangci-lint run ./...` + Integration (s.o.) — alles grün.
2. `README.md`-Testzähler (151) auf den neuen Stand anheben (Zählen via `go test ./... -v | grep -c -- "--- PASS"`).
3. In `improvements.md` die Items #1–#10 (Quick Wins) und #11-Teilumfang als erledigt markieren (Häkchen + Commit-Referenz).
4. Abschließender Diff-Review: `git diff main --stat` — nur erwartete Dateien; keine unverbundenen Änderungen.

**Explizit NICHT Teil dieses Plans:** Metriken/Observability (#12), Ack-Deadline-Management (#13), AuthN/AuthZ (#14), Quota-Aggregation (#15), KV-Watch (#16), alle Strategic Items (#17–#20). Diese erfordern Architektur-Entscheidungen und folgen in einem separaten Plan.
