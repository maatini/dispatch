# Testabdeckung auf sehr gutes Niveau heben

## Ziel

Alle Packages auf ≥ 80 % Coverage heben, Fokus auf die Lücken: **admin 13 %, msgraph 47 %, worker 55 %, natsutil 0 %**. Dazu zwei Verhaltens-Bugs fixen (fail-open Dedup, uploadChunks-Fehlerklassifizierung) und minimal-invasive Testbarkeits-Refactorings.

## Test-Konventionen (aus Bestandscode übernehmen)

- White-Box (gleiches Package), kein testify — nur Stdlib
- Naming: `Test<Func>_<Scenario>`; `t.Run` nur bei echten Tabellen
- Mocks: `stubX{err error}` für Consumer-Interfaces, `mockKV` (map-basiert) + `mockEntry` für NATS-KV, `captureJS`/`capturePublisher` zum Aufzeichnen, `failX` für Fehlerpfade
- Konstruktion per Struct-Literal `&Type{dep: stub}`; externe APIs via `httptest.NewServer` mit Capture-Vars
- `errors.As` für typisierte Fehler; `t.Helper()` in t-nehmenden Helpern; Testdaten-Konstanten oben in der Datei
- Grenzfälle mutation-resistent wählen (Off-by-one, Vorzeichen)

## Phase 1: Testbarkeits-Refactorings + Bugfixes (Produktionscode)

### 1.1 `internal/msgraph/service.go` — injizierbares baseURL
- `Service`-Struct um Feld `baseURL string` ergänzen (Muster exakt aus `internal/msgraph/bounce.go` L17/L24–28 kopieren)
- Konstruktor setzt Default = bisherige Package-Konstante; URLs in `sendInline` (L52), `createDraft`, `addSmallAttachment`, `uploadLargeAttachment` auf `s.baseURL` umstellen
- Ermöglicht httptest-Server-Tests für die gesamte Send-Pipeline

### 1.2 `internal/msgraph/client.go` — injizierbares Retry-Delay
- `fallbackDelay = 2s` (L132) als Feld `retryBaseDelay time.Duration` am `Client` (Default 2s im Konstruktor); Tests setzen es auf ~1ms
- Ermöglicht `doWithRetry`-Tests ohne reale Sleeps

### 1.3 Bugfix: `internal/worker/processor.go` L75–79 — Dedup fail-open → fail-closed
- Aktuell: jeder KV-Fehler bei `delivered.Get(traceID)` wird als „nicht zugestellt" interpretiert → Doppelversand-Risiko
- Neu: `nats.ErrKeyNotFound` = nicht zugestellt (weiter verarbeiten); jeder andere Fehler = transiente Behandlung (return ohne Ack → JetStream-Redelivery), kein Graph-Call
- Konsistent zur Fail-closed-Philosophie des Projekts

### 1.4 Bugfix: `internal/msgraph/service.go` `uploadChunks` (~L190) — 4xx nicht mehr transient
- Aktuell: jeder Status ≥ 400 → `GraphTransientError` → Endlos-Redelivery bei permanenten Fehlern (z.B. abgelaufene Upload-URL)
- Neu: 429/5xx → `GraphTransientError`, übrige 4xx → `GraphPermanentError` (Klassifizierung analog `client.do` L110–116)

## Phase 2: Unit-Tests

### 2.1 `internal/msgraph` (Ziel ~85 %)

**`ratelimiter_test.go` (NEU — komplett ungetestet, trivial testbar):**
- `Wait` mit `skipWait=true` → sofort nil
- Per-Sender-Isolation: gleicher Sender → gleicher Limiter, verschiedene Sender → verschiedene (via `get`)
- Burst-Logik: 10 sofortige Calls ok, 11. blockiert → via `ctx`-Cancellation testen (keine realen Sleeps)

**`client_test.go` (erweitern):**
- `do`: getToken-Fehler propagiert; 429 → `RetryAfter` im Fehler; 5xx → `GraphTransientError`; 4xx → `GraphPermanentError` mit Body
- `doWithRetry` (mit injiziertem 1ms-Delay): Erfolg im 2. Versuch; max retries exceeded; sofortiger Abbruch bei permanentem Fehler (callCount == 1); `ctx.Done()` während Backoff
- `errors_test.go` (NEU, trivial): `Error()`/`Unwrap()` beider Fehlertypen

**`service_test.go` (erweitern, nach Refactoring 1.1):**
- `SendEmail`: Threshold-Verzweigung (totalSize < 3 MiB → inline via `sendMail`-Pfad; ≥ 3 MiB → Upload-Session-Pfad, am Request-URL des httptest-Servers unterscheidbar); RateLimiter-Fehler propagiert (Service mit `NewRateLimiter(false)` + erschöpftem Limiter oder skipWait=false + ctx)
- `sendViaUploadSession`: createDraft-Fehler → Abbruch; Small/Large-Verzweigung pro Attachment (`inlineThresholdBytes`); Attachment-Fehler → cleanup (DELETE-Call am Server beobachten); finaler Send-Fehler → cleanup
- `createDraft`: Invalid-JSON-Antwort → Parse-Fehler
- `uploadChunks`: 400 → `GraphPermanentError` (dokumentiert Bugfix 1.4), 429/500 → transient, Netzwerkfehler (Server geschlossen) → `GraphTransientError`
- `buildGraphEmail`: HTML- vs. Text-Body, CC/BCC-Mapping, Attachment-Einbettung bei `includeAttachments=true`

**`token_test.go` (erweitern):**
- `fetchToken`: Netzwerkfehler → `GraphTransientError` (Server vor Request schließen)
- `tokenCache.get`: Fehlerpropagation bei fehlgeschlagenem Refresh

### 2.2 `internal/worker` (Ziel ~85 %)

**`attachstore_test.go` (NEU — via Fake-Interfaces, kein NATS nötig):**
- Fake `nats.ObjectStore` + Fake `nats.ObjectResult` (beides Interfaces)
- `Fetch`: Passthrough bei leerem `ObjectKey`; `Get`-Fehler → wrapped error; `ReadCloser`-Fehler mid-read → wrapped error; Erfolgspfad
- `Cleanup`: Skip bei leerem `ObjectKey`; `Delete`-Fehler → kein Panik/Abbruch (nur Warn)

**`processor_test.go` (erweitern):**
- Dedup-KV-Fehler (nicht `ErrKeyNotFound`) → fail-closed: kein Graph-Call, kein Audit, Fehlerbehandlung (dokumentiert Bugfix 1.3); `stubKV` um `getErr`-Feld erweitern
- `delivered.Put`-Fehler → Warn, Verarbeitung läuft weiter (kein Panik)

### 2.3 `internal/admin` (Ziel ~80 %)

**`resolver_test.go` (erweitern — reine Funktionen ohne Infrastruktur):**
- `matchesMailFilter`: nil-Filter, AppTag/Status/TraceID einzeln und kombiniert (AND)
- `pageSize`: Defaults (nil, 0, negativ → 50), valider Wert
- `paginate`: Fensterung, `start >= total` → nil+total, letzte Teiler-Seite
- Mapper: `toSenderGQL`/`fromSenderInput` (AllowedDomains ""↔nil), `toMailRecordGQL` (Subject/Error nur wenn ≠ ""), `toBounceRecordGQL`, `toDeadLetterGQL`

**Sender-CRUD (mit mockKV-basiertem `sender.Store`, Muster aus `internal/sender/sender_test.go`):**
- `Senders`: Liste + AppTag-Filter; `CreateSender`: Put + Roundtrip; `UpdateSender`: AppTag aus Pfadargument überschreibt Input; `DeleteSender`: true + Eintrag weg; KV-Fehler propagiert

**`server_test.go` (NEU):**
- `NewHTTPHandler` mit echtem Resolver → Schema-Parsing erfolgreich (fängt Schema/Resolver-Drift beim Test statt zur Laufzeit)

### 2.4 `internal/natsutil` (Ziel ~80 %, eingebetteter nats-server)

**Vorbereitung:** `go.mod` um `github.com/nats-io/nats-server/v2` ergänzen (nur in Tests verwendet)

**`setup_test.go` (NEU):**
- Helper `testNATS(t)`: `server.NewServer` mit `JetStream: true, Port: -1, StoreDir: t.TempDir()`, `t.Cleanup(server.Shutdown)`, liefert JetStreamContext
- `Connect`: ungültige URL → Fehler
- `ProvisionStreams`: legt 4 Streams an (Namen/Subjects via `StreamInfo` assertieren); zweiter Aufruf idempotent; **Update-Pfad**: Stream mit geänderter Config → `UpdateStream` wird aufgerufen
- `ProvisionKVBuckets`: Buckets existieren; `spamTTL` propagiert (`KeyValue` → `BucketInfo` TTL prüfen)
- `ProvisionObjectStore`: Bucket wird angelegt, zweiter Aufruf liefert bestehenden
- `ProvisionWorkerConsumer`: Consumer wird angelegt (`ConsumerInfo` assertieren); zweiter Aufruf idempotent

## Phase 3: Integrationstests erweitern (Build-Tag `integration`)

**`internal/admin/integration_test.go`:**
- `Mails` mit Filter (AppTag/Status) und Pagination (page/size, total)
- `Bounces` und `DeadLetters` Happy Path (Records publizieren, lesen, korrupte überspringen)
- `readStream`-Fehlerpfad: nicht-existenter Stream → Fehler

**`internal/worker/integration_test.go`:**
- `TestIntegration_ConsumerRun`: echter Stream + `Consumer.Run` mit echtem Processor + `transientGraphStub` (Muster `recordingPublisher` wiederverwenden); verifiziert Fetch→Handle→Redelivery-Pfad inkl. Consumer-Loop (deckt `consumer.go` ab)

## Betroffene Dateien

| Aktion | Datei |
|---|---|
| Refactor | `internal/msgraph/service.go`, `internal/msgraph/client.go` |
| Bugfix | `internal/worker/processor.go`, `internal/msgraph/service.go` |
| Neue Tests | `internal/msgraph/ratelimiter_test.go`, `internal/msgraph/errors_test.go`, `internal/worker/attachstore_test.go`, `internal/admin/server_test.go`, `internal/natsutil/setup_test.go` |
| Erweiterte Tests | `internal/msgraph/client_test.go`, `internal/msgraph/service_test.go`, `internal/msgraph/token_test.go`, `internal/worker/processor_test.go`, `internal/admin/resolver_test.go`, `internal/admin/integration_test.go`, `internal/worker/integration_test.go` |
| Dependency | `go.mod` (+ `nats-server/v2`) |

## Verifikation

1. `go build ./...` und `golangci-lint run` (Konfig `.golangci.yml` vorhanden)
2. `go test ./... -race -coverprofile=coverage.out` — Coverage pro Package prüfen: admin/msgraph/worker/natsutil ≥ 80 %, keine Regression in Bestands-Packages
3. `go test -tags integration ./... -race` mit lokalem NATS (`devbox run nats:up` bzw. docker-compose)
4. `devbox run mutate` (gremlins, Schwellwerte 70 % Efficacy/Coverage) auf den geänderten Packages — neue Tests sollen Mutanten töten
5. Bestehende CI-Pipeline (`build.yml`) muss grün bleiben
