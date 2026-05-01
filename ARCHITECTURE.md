# Architektur — dispatch

Dispatch ist ein mandantenfähiges E-Mail-Delivery-System. Eine REST-Schnittstelle nimmt Versandaufträge entgegen, leitet sie über NATS JetStream an einen Worker weiter, der die E-Mail über die MS Graph API zustellt.

---

## Services

```
┌─────────────────┐     ┌──────────────────┐     ┌───────────────────┐
│  mail-gateway   │     │   mail-worker    │     │    mail-admin     │
│                 │     │                  │     │                   │
│ POST /mail/send │────▶│ NATS Consumer    │────▶│ GraphQL API       │
│ 7-Stage Pipeline│     │ MS Graph Send    │     │ Sender-CRUD       │
│                 │     │ Audit / DLQ      │     │ Stream-Queries    │
└─────────────────┘     └──────────────────┘     └───────────────────┘
         │                       │
         │                       │
         ▼                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          NATS JetStream                             │
│                                                                     │
│  Streams                       KV Buckets           Object Store   │
│  ─────────────────────         ────────────         ────────────── │
│  DISPATCH_MAILS (72h)          senders              attachments    │
│  DISPATCH_AUDIT (30d)          quota (25h TTL)      (72h TTL)     │
│  DISPATCH_DEAD_LETTERS (30d)   spam (60s TTL)                      │
│  DISPATCH_BOUNCES (30d)        delivered (7d TTL)                  │
└─────────────────────────────────────────────────────────────────────┘
         ▲
         │
┌─────────────────┐
│  bouncemanage-  │
│     ment        │
│ 15-min Crawler  │
│ NDR → Bounce-   │
│ Record          │
└─────────────────┘
```

| Service | Einstiegspunkt | Primäre Aufgabe |
|---|---|---|
| `mail-gateway` | `POST /dispatch/api/v1/mail/send` | Validierung, Quota, Spam-Dedup, NATS-Publish |
| `mail-worker` | NATS Pull-Consumer `mail-worker` | E-Mail-Versand via MS Graph, Audit, Dead-Letter |
| `mail-admin` | GraphQL `/graphql` | Sender-Verwaltung, Stream-Abfragen, Reprocessing |
| `bouncemanagement` | Ticker (15 min) | NDR-Crawler, Trace-ID-Extraktion, Bounce-Records |

---

## Mail-Versand: Datenfluss

```
HTTP POST /dispatch/api/v1/mail/send
        │
        ▼
┌───────────────────────────────────┐
│       7-Stage Gateway Pipeline    │
│                                   │
│  1  JSON-Decode + Struct-Validier.│
│     (validator, MIME-Whitelist,   │
│      Größenlimits)                │
│                                   │
│  2  Sender-Lookup (appTag → KV)   │
│     ┌─ Cache (10 min) ────┐       │
│     └─ NATS KV senders ──┘       │
│                                   │
│  3  Domain-Whitelist              │
│     (AllowedDomains pro Sender)   │
│                                   │
│  4  Quota-Check (rolling 24h)     │
│     CAS-Loop (max 10 Retries)     │
│     Fail-closed: KV-Fehler → 503  │
│                                   │
│  5  Spam-Dedup (SHA-256)          │
│     appTag|subject|recip|size     │
│     NATS KV spam (60s TTL)        │
│                                   │
│  6  Anhang-Upload                 │
│     decode base64 → Object Store  │
│     Fehler → HTTP 503             │
│                                   │
│  7  NATS Publish → DISPATCH_MAILS │
│     Fehler → HTTP 503             │
│     Erfolg → HTTP 202             │
└───────────────────────────────────┘
```

```
NATS Consumer (pull, explicit ACK, 30s ack-wait)
        │
        ▼
┌───────────────────────────────────┐
│          Processor.Handle         │
│                                   │
│  JSON-Parse fehlt → Dead Letter   │
│         + ACK                     │
│                                   │
│  Duplicate (delivered KV) → ACK   │
│                                   │
│  Attachments: Object Store Fetch  │
│  Fehler → kein ACK (Redelivery)   │
│                                   │
│  Test-Flag → Audit TEST_SUCCESS   │
│              + ACK + Cleanup      │
│                                   │
│  MS Graph SendEmail               │
│  ┌─ Transient (429/5xx/IO) ──────┐│
│  │  kein ACK → JetStream         ││
│  │  redelivert                   ││
│  └───────────────────────────────┘│
│  ┌─ Permanent (4xx) ─────────────┐│
│  │  Audit FAILED + ACK + Cleanup ││
│  └───────────────────────────────┘│
│  ┌─ Erfolg ──────────────────────┐│
│  │  Audit DELIVERED              ││
│  │  delivered KV schreiben       ││
│  │  ACK + Object-Store Cleanup   ││
│  └───────────────────────────────┘│
└───────────────────────────────────┘
```

---

## NATS-Topologie: Wer liest/schreibt was

| Ressource | Gateway | Worker | Admin | Bouncemanagement |
|---|---|---|---|---|
| `DISPATCH_MAILS` | **publish** | **consume** | reprocess (publish) | — |
| `DISPATCH_AUDIT` | — | **publish** | read | — |
| `DISPATCH_DEAD_LETTERS` | — | **publish** | read | — |
| `DISPATCH_BOUNCES` | — | — | read | **publish** |
| KV `senders` | read (cache) | — | **read/write** | — |
| KV `quota` | **read/write** (CAS) | — | — | — |
| KV `spam` | **read/write** | — | — | — |
| KV `delivered` | — | **read/write** | — | — |
| Object Store `attachments` | **put** | **get/delete** | — | — |

---

## MS Graph Integration

### E-Mail-Versand (`msgraph.Service`)

```
SendEmail(req)
    │
    ▼
Rate Limiter (per Sender, 1 req/s, Burst 10)
    │
    ▼
Gesamtgröße Attachments?
    │
    ├─ < 3 MB ──▶ sendInline
    │              POST /users/{sender}/sendMail
    │              (Attachments base64-embedded)
    │
    └─ ≥ 3 MB ──▶ sendViaUploadSession
                   POST /users/{sender}/messages      (Draft)
                   POST .../attachments               (< 3 MB je Anhang)
                   POST .../attachments/createUploadSession  (≥ 3 MB)
                     └─ PUT chunks (1,25 MB je Chunk)
                   POST .../messages/{id}/send
```

### NDR-Crawling (`msgraph.BounceService`)

```
BounceService.GetUnreadMessages(mailbox)
    │
    ▼
GET /users/{mailbox}/messages?$filter=isRead+eq+false&$select=id,subject,body
    │
    ▼
Parse → []NDRMessage{ID, Subject, Body}
    │
    ▼
Crawler.process → Trace-ID extrahieren → DISPATCH_BOUNCES
    │
    ▼
BounceService.MarkAsRead(mailbox, messageID)
    │
    ▼
PATCH /users/{mailbox}/messages/{id}   {"isRead": true}
```

**Fehler-Handling im HTTP-Client:**

| HTTP-Status | Fehlertyp | Verhalten |
|---|---|---|
| 429 | `GraphTransientError` + `RetryAfter` | Retry nach `Retry-After`-Header (max 30 s), max 3 Versuche |
| 5xx | `GraphTransientError` | Retry mit 2 s Fallback-Delay |
| 4xx (≠ 429) | `GraphPermanentError` | Kein Retry, zählt nicht gegen Circuit Breaker |
| IO-Fehler | `GraphTransientError` | Retry |
| 5 konsekutive Fehler | Circuit Breaker öffnet | 30 s Pause, dann Half-Open |

---

## Fehler-Semantik (Gateway → HTTP)

| Fehler | HTTP | Auslöser |
|---|---|---|
| Request-Body überschreitet Limit | 413 | `http.MaxBytesReader` vor JSON-Decode |
| Validierungsfehler (Format, MIME, Größe) | 400 | Stage 1 |
| Unbekannter `appTag` | 400 | Stage 2 |
| Domain nicht erlaubt | 400 | Stage 3 |
| Quota überschritten | 429 + `X-RateLimit-*` | Stage 4 |
| KV-Fehler bei Quota | 503 | Stage 4 (fail-closed) |
| Spam-Duplikat | 400 | Stage 5 |
| Object-Store-Fehler | 503 | Attachment-Upload |
| NATS-Publish-Fehler | 503 | Publish |

---

## Resilienz

**Quota:** Fail-closed. Jeder KV-Fehler → HTTP 503, kein Bypass. Optimistic CAS mit max. 10 Retries; nach Erschöpfung → `QuotaStateError`.

**Worker-Idempotenz:** `delivered` KV (7-Tage-TTL) verhindert Doppelversand bei Worker-Absturz nach Graph-Erfolg und vor ACK.

**Attachments:** NATS Object Store entkoppelt Payload-Größe vom JetStream-Limit. Bucket-TTL (72 h) bereinigt Waisen-Objekte nach Worker-Crash ohne Cleanup.

**Bounce-Matching:** `BounceService` (MS Graph) ruft alle 15 Minuten ungelesene Nachrichten aus der Bounce-Mailbox ab, extrahiert die Trace-ID via `X-Dispatch-TraceId`-Header im NDR-Body und schreibt einen `BounceRecord` nach `DISPATCH_BOUNCES`. Verarbeitete Nachrichten werden via `PATCH .../messages/{id}` als gelesen markiert.

**Attachment-Streaming:** Base64-Inhalt von Anhängen wird im Gateway nie vollständig als `[]byte` dekodiert. Validierung (Größe, Formatprüfung) und Upload in den NATS Object Store erfolgen durch Streaming via `base64.NewDecoder` — O(1) Speicher unabhängig von der Anhangsgröße.

---

## Konfiguration

Alle Werte kommen aus Umgebungsvariablen. Keine Config-Dateien.

**Pflichtfelder** (ohne die kein Start):
```
NATS_URL
MS_GRAPH_TENANT_ID           \
MS_GRAPH_CLIENT_ID            } entfallen wenn MS_GRAPH_MOCK_TOKEN gesetzt
MS_GRAPH_CLIENT_SECRET       /
MS_GRAPH_SENDER_EMAIL
DISPATCH_ADMIN_AUTH_SECRET   # HMAC-Schlüssel für Admin-API JWT-Auth
```

**Optionale Felder (Auswahl):**
```
PORT=8080
DISPATCH_SPAM_TIMEOUT_SECONDS=60
DISPATCH_VALIDATION_MAX_BODY_SIZE=10000000
DISPATCH_MAX_TOTAL_ATTACHMENT_SIZE_MB=20
DISPATCH_GRAPH_RATE_LIMITER_SKIP_SLEEP=false
MS_GRAPH_PROXY_URL=           # Dev Proxy (http://localhost:8000)
MS_GRAPH_MOCK_TOKEN=          # Überspringt OAuth2, macht Credentials optional
```

---

## Entwicklungsumgebung

```bash
devbox run up-proxy        # NATS + MS Graph Dev Proxy (Port 8000)
devbox run run-worker-dev  # Worker ohne echte MS-Graph-Credentials
devbox run run-gateway-dev # Gateway lokal

devbox run test            # Unit-Tests (kein Docker nötig)
devbox run lint            # golangci-lint
devbox run coverage-html   # HTML-Coverage-Report → coverage.html
devbox run mutate          # Mutations-Tests (gremlins) auf Core-Packages
devbox run metrics         # Coverage + Mutation in einem Lauf
devbox run sonar           # Coverage erzeugen + SonarQube-Scan
```

Der MS Graph Developer Proxy (`ghcr.io/dotnet/dev-proxy:latest`) mockt alle genutzten Graph-Endpunkte. Konfiguration in `dev-proxy/devproxyrc.json`, Mock-Antworten in `dev-proxy/mocks.json`.
