# dispatch

Multi-tenantes E-Mail-Delivery-System. REST-Eingang → NATS JetStream → Microsoft Graph API.

## Architektur

```
Client
  POST /codymail/api/v1/mail/send
  └── mail-gateway
        1. Validierung (Format, Größe, MIME)
        2. Sender-Lookup (NATS KV senders, In-Memory-Cache 10 min)
        3. Domain-Whitelist-Check
        4. Quota-Check (NATS KV quota, rolling 24h, CAS, fail-closed)
        5. Spam-Deduplizierung (NATS KV spam, TTL-Bucket)
        6. Publish → NATS JetStream CODYMAIL_MAILS
           ↳ Fehler → HTTP 503 (kein Retry, kein Fallback)
           ↳ Erfolg → HTTP 202

  NATS JetStream
  └── mail-worker (durable Pull-Consumer)
        1. JSON-Deserialisierung (→ CODYMAIL_DEAD_LETTERS bei Fehler)
        2. Dedup via NATS KV delivered (7-Tage-TTL)
        3. Test-Modus: Audit-Eintrag ohne MS-Graph-Call
        4. sendMail / Upload-Session via MS Graph API
           ↳ 429/5xx → kein ACK, JetStream redelivert
           ↳ 4xx      → ACK + FAILED in CODYMAIL_AUDIT
           ↳ Erfolg   → ACK + DELIVERED in CODYMAIL_AUDIT

  mail-admin    → GraphQL-API: Sender-Verwaltung, Audit-Log, Dead-Letters
  bouncemanagement → MS-Graph-Poller (alle 15 min) → CODYMAIL_BOUNCES
```

**State-Backend: ausschließlich NATS** — kein PostgreSQL, kein Redis, kein externes System.

## Services

| Service | Endpunkt | Zweck |
|---------|----------|-------|
| `cmd/mail-gateway` | `POST /codymail/api/v1/mail/send` | HTTP-Eingang, Validierung, Publish |
| `cmd/mail-worker` | — | NATS-Consumer, MS-Graph-Delivery |
| `cmd/mail-admin` | `POST /graphql` | Sender-CRUD, Audit-Abfragen |
| `cmd/bouncemanagement` | — | NDR-Crawler, Bounce-Aufzeichnung |

## NATS-Ressourcen

| Typ | Name | Zweck |
|-----|------|-------|
| KV | `senders` | Sender-Konfiguration (appTag → Email, Quota, Domains) |
| KV | `quota` | Rolling-24h-Verbrauch pro appTag (optimistic CAS) |
| KV | `spam` | SHA-256-Fingerprints mit TTL-Ablauf |
| KV | `delivered` | Dedup-Index für Worker (7-Tage-TTL) |
| Stream | `CODYMAIL_MAILS` | Work-Queue (WorkQueuePolicy, 72h Retention) |
| Stream | `CODYMAIL_AUDIT` | Delivery-Ergebnisse (DELIVERED / FAILED / TEST_SUCCESS) |
| Stream | `CODYMAIL_DEAD_LETTERS` | Nicht-parsbare Nachrichten |
| Stream | `CODYMAIL_BOUNCES` | NDR-Ergebnisse aus Bounce-Crawler |

## Konfiguration

### Pflicht (kein Default — Service startet nicht ohne diese)

```
NATS_URL
MS_GRAPH_TENANT_ID
MS_GRAPH_CLIENT_ID
MS_GRAPH_CLIENT_SECRET
MS_GRAPH_SENDER_EMAIL
```

### Optional

```
PORT=8080
MS_GRAPH_BOUNCE_MAILBOX           # default: MS_GRAPH_SENDER_EMAIL
CODYMAIL_SPAM_TIMEOUT_SECONDS=60
CODYMAIL_VALIDATION_MAX_BODY_SIZE=10000000
CODYMAIL_VALIDATION_MIME_WHITELIST=application/pdf,image/jpeg,image/png,...
CODYMAIL_MAX_TOTAL_ATTACHMENT_SIZE_MB=20
CODYMAIL_NATS_PUBLISH_TIMEOUT_SECONDS=5
CODYMAIL_GRAPH_RATE_LIMITER_SKIP_SLEEP=false
```

## Lokale Entwicklung

```bash
# Voraussetzung: devbox (https://www.jetpack.io/devbox)
devbox shell

# NATS starten
devbox run up

# Services einzeln starten (in separaten Terminals)
export $(grep -v '^#' .env.local | xargs)
go run ./cmd/mail-gateway
go run ./cmd/mail-worker
go run ./cmd/mail-admin
go run ./cmd/bouncemanagement
```

Beispiel `.env.local` (nicht einchecken):

```bash
NATS_URL=nats://localhost:4222
MS_GRAPH_TENANT_ID=<azure-tenant-id>
MS_GRAPH_CLIENT_ID=<client-id>
MS_GRAPH_CLIENT_SECRET=<client-secret>
MS_GRAPH_SENDER_EMAIL=noreply-dev@example.com
CODYMAIL_SPAM_TIMEOUT_SECONDS=5
CODYMAIL_GRAPH_RATE_LIMITER_SKIP_SLEEP=true
```

NATS Monitoring: http://localhost:8222

## Build & Test

```bash
devbox run build          # go build ./...
devbox run test           # alle Unit-Tests
devbox run test-gateway   # nur Gateway
devbox run test-worker    # nur Worker
devbox run lint           # golangci-lint
devbox run coverage       # Tests + Coverage-Report
devbox run test-integration  # Integrationstests (Docker erforderlich)
```

## API

### Mail senden

```
POST /codymail/api/v1/mail/send
Content-Type: application/json

{
  "appTag": "alv-dev",
  "recipients": ["user@example.com"],
  "ccRecipients": [],
  "bccRecipients": [],
  "subject": "Hello",
  "bodyContent": "Plain text body",
  "htmlBodyContent": "<p>HTML body</p>",
  "attachments": [
    {
      "name": "file.pdf",
      "mimeType": "application/pdf",
      "content": "<base64>"
    }
  ],
  "traceContext": {}
}
```

**Antworten:**

| Status | Bedeutung |
|--------|-----------|
| `202` | Nachricht an NATS übergeben |
| `400` | Validierungsfehler (Pflichtfelder, Domain, Spam, MIME) |
| `429` | Tages-Quota überschritten (`X-RateLimit-Limit`, `X-RateLimit-Remaining`) |
| `503` | NATS nicht erreichbar oder Quota-State-Fehler (Client soll wiederholen) |

### Health

```
GET /health        → {"status":"UP","checks":[...]}
GET /health/live   → 200
GET /health/ready  → 200
```

### Admin GraphQL

```
POST /graphql

# Sender anlegen
mutation {
  createSender(input: {
    appTag: "alv-dev"
    email: "noreply@example.com"
    test: false
    dailyQuota: 1000
    allowedDomains: "example.com,partner.de"
  }) { appTag email dailyQuota }
}

# Audit-Log abfragen
query {
  mails(filter: { appTag: "alv-dev", status: "DELIVERED" }, page: 0, size: 20) {
    total
    items { traceId status timestamp recipients }
  }
}
```

## Resilience-Verhalten

| Fehlerfall | Verhalten |
|------------|-----------|
| NATS beim Publish nicht erreichbar | HTTP 503, kein Retry im Gateway |
| Quota KV-Fehler | HTTP 503 (fail-closed, niemals bypass) |
| MS Graph 429 / 5xx | Kein NATS-ACK, JetStream redelivert |
| MS Graph 4xx (außer 429) | ACK, FAILED in Audit |
| JSON-Parse-Fehler im Worker | ACK, Dead-Letter-Stream |
| Worker-Absturz nach Graph-Erfolg | Dedup via KV `delivered` verhindert Doppelversand |
| E-Mail-Adressen in Logs | Immer maskiert: `u***@domain.com` |

## Stack

- **Go 1.24+**
- **NATS JetStream** — Message-Broker, KV-Store, State-Backend
- **Microsoft Graph API v1.0** — E-Mail-Versand via Microsoft 365
- **`github.com/go-chi/chi/v5`** — HTTP-Routing
- **`github.com/go-playground/validator/v10`** — Request-Validierung
- **`github.com/nats-io/nats.go`** — NATS-Client
- **`github.com/sony/gobreaker`** — Circuit Breaker (MS Graph)
- **`golang.org/x/time/rate`** — Token-Bucket-Rate-Limiter pro Sender
- **`github.com/graph-gophers/graphql-go`** — GraphQL (Admin-API)
- **`log/slog`** — Strukturiertes JSON-Logging
