# mail-worker: Interfaces

## NATS Subject Contracts

| Subject | Direction | Payload |
|---|---|---|
| `cody.mailing.job.request.mails` | Consume | `domain.MailRequestDO` (JSON) |
| `cody.mailing.audit` | Publish | `domain.AuditRecord` (JSON) |
| `cody.mailing.deadletter` | Publish | `domain.DeadLetter` (JSON) |

## MailRequestDO (consumed from NATS)

```go
type MailRequestDO struct {
    TraceID         string            `json:"traceId"`
    AppTag          string            `json:"appTag"`
    Sender          string            `json:"sender"`
    Recipients      []string          `json:"recipients"`
    CcRecipients    []string          `json:"ccRecipients,omitempty"`
    BccRecipients   []string          `json:"bccRecipients,omitempty"`
    Subject         string            `json:"subject,omitempty"`
    BodyContent     string            `json:"bodyContent,omitempty"`
    HtmlBodyContent string            `json:"htmlBodyContent,omitempty"`
    Attachments     []AttachmentDO    `json:"attachments,omitempty"`
    TraceContext    map[string]string `json:"traceContext,omitempty"`
    Test            bool              `json:"test"`
}

type AttachmentDO struct {
    Name        string `json:"name"`
    ContentType string `json:"contentType"`
    ObjectKey   string `json:"objectKey,omitempty"` // populated by gateway
    Content     []byte `json:"-"`                   // populated by worker at runtime
}
```

## AuditRecord (published to audit stream)

```go
type AuditRecord struct {
    TraceID    string    `json:"traceId"`
    AppTag     string    `json:"appTag"`
    Status     string    `json:"status"` // DELIVERED | FAILED | TEST_SUCCESS
    Sender     string    `json:"sender"`
    Subject    string    `json:"subject"`
    Recipients []string  `json:"recipients"`
    Error      string    `json:"error,omitempty"`
    Timestamp  time.Time `json:"timestamp"`
}
```

## DeadLetter (published to dead letter stream)

```go
type DeadLetter struct {
    Payload   string    `json:"payload"`
    Error     string    `json:"error"`
    Timestamp time.Time `json:"timestamp"`
}
```

## Consumer-Side Interfaces

```go
type emailSender interface {
    SendEmail(ctx context.Context, req domain.MailRequestDO) error
}

type deliveredStore interface {
    Get(key string) (nats.KeyValueEntry, error)
    Put(key string, value []byte) (uint64, error)
}

type jsPublisher interface {
    Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

type attachmentFetcher interface {
    Fetch(attachments []domain.AttachmentDO) ([]domain.AttachmentDO, error)
    Cleanup(attachments []domain.AttachmentDO)
}
```

## Consumer Configuration

- **Durable name:** `mail-worker`
- **Ack policy:** `AckExplicit`
- **Ack wait:** 30 seconds
- **Max deliver:** unlimited (-1)
- **Filter subject:** `cody.mailing.job.request.mails`
- **Fetch batch size:** 10 messages
