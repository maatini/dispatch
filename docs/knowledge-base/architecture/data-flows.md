# Data Flows

## Happy Path: Send Mail

```mermaid
sequenceDiagram
    actor Client
    participant GW as mail-gateway<br/>(gateway.Handler)
    participant Sender as sender.Store<br/>(NATS KV + Cache)
    participant Quota as quota.Checker<br/>(NATS KV, CAS)
    participant Spam as spam.Checker<br/>(NATS KV, TTL)
    participant AttStore as gateway.AttachmentStore<br/>(NATS Object Store)
    participant Pub as gateway.NatsPublisher
    participant NATS as NATS JetStream
    participant Worker as mail-worker<br/>(worker.Processor)
    participant MSGraph as msgraph.Service
    participant Delivered as delivered KV
    participant Audit as DISPATCH_AUDIT<br/>Stream

    Client->>GW: POST /dispatch/api/v1/mail/send<br/>{appTag, recipients, subject, ...}
    GW->>GW: 1. JSON decode → domain.MailRequest
    GW->>GW: 2. Struct validation (validator, MIME, size, base64)
    GW->>Sender: 3. Get(appTag) → domain.Sender
    Sender-->>GW: sender (cached or KV)
    GW->>GW: 4. Domain whitelist check
    GW->>Quota: 5. Check(appTag, limit, count)
    Quota-->>GW: OK or QuotaError
    GW->>Spam: 6. Check(hash)
    Spam-->>GW: OK or ValidationError
    GW->>AttStore: 7. Upload(attachments) — streaming base64 decode
    AttStore-->>GW: []AttachmentDO (with ObjectKeys)

    GW->>Pub: Publish(MailRequestDO)
    Pub->>NATS: JetStream publish → DISPATCH_MAILS
    GW-->>Client: 202 Accepted {traceId}

    note over NATS,Worker: Pull consumer fetches batch of 10

    Worker->>Worker: JSON unmarshal MailRequestDO
    alt malformed JSON
        Worker->>Audit: write dead letter
        Worker->>NATS: ACK
    end

    Worker->>Delivered: Get(traceID)
    alt traceID exists
        Worker->>NATS: ACK (duplicate, skip)
    end

    Worker->>AttStore: Fetch attachments ← Object Store
    AttStore-->>Worker: []byte content

    alt test mode (sender.Test == true)
        Worker->>Delivered: Put(traceID, "1")
        Worker->>Audit: publish TEST_SUCCESS
        Worker->>AttStore: Cleanup (delete objects)
        Worker->>NATS: ACK
    end

    Worker->>MSGraph: SendEmail(req)
    alt success
        MSGraph-->>Worker: OK
        Worker->>Delivered: Put(traceID, "1")
        Worker->>Audit: publish DELIVERED
        Worker->>AttStore: Cleanup
        Worker->>NATS: ACK
    else transient error (429/5xx)
        MSGraph-->>Worker: GraphTransientError
        note over Worker: NO ACK → JetStream redelivers
    else permanent error (4xx≠429)
        MSGraph-->>Worker: GraphPermanentError
        Worker->>Audit: publish FAILED
        Worker->>AttStore: Cleanup
        Worker->>NATS: ACK
    end
```

## Bounce Detection

```mermaid
sequenceDiagram
    participant Ticker as Ticker (15 min)
    participant Crawler as bounce.Crawler
    participant BounceSvc as msgraph.BounceService
    participant MSGraph as MS Graph API
    participant NATS as NATS JetStream
    participant Admin as mail-admin (read-only)

    Ticker->>Crawler: Run(ctx)
    Crawler->>BounceSvc: GetUnreadMessages(mailbox)
    BounceSvc->>MSGraph: GET /users/{mailbox}/messages<br/>?$filter=isRead eq false
    MSGraph-->>BounceSvc: []NDRMessage {ID, Subject, Body}

    loop each NDR message
        Crawler->>Crawler: extractTraceID(body)<br/>regex: X-Dispatch-TraceId: <uuid>
        Crawler->>NATS: Publish BounceRecord → DISPATCH_BOUNCES
        Crawler->>BounceSvc: MarkAsRead(mailbox, messageID)
        BounceSvc->>MSGraph: PATCH /users/{mailbox}/messages/{id}<br/>{"isRead": true}
    end

    note over Admin,NATS: mail-admin reads bounce records via temporary subscription

    Admin->>NATS: SubscribeSync(DISPATCH_BOUNCES, DeliverAll)
    NATS-->>Admin: []BounceRecord
```

## Key Data Transformations

| Transform | Where | Input → Output |
|---|---|---|
| `MailRequest` → `MailRequestDO` | mail-gateway handler | Inline attachment base64 → Object Store keys; adds traceID, sender email, test flag |
| `AttachmentDO` (ObjectKey) → `AttachmentDO` (Content) | mail-worker processor | Object Store → []byte; `json:"-"` field populated at runtime |
| Base64 → raw bytes (streaming) | gateway attachment upload | `base64.NewDecoder` → writes directly to Object Store (O(1) memory) |
| MS Graph JSON → `domain.AuditRecord` | mail-worker processor | Delivery outcome marshaled to NATS stream message |
| NDR body → `X-Dispatch-TraceId` | bounce crawler | Regex extraction: `X-Dispatch-TraceId:\s*([0-9a-f-]{36})` |
