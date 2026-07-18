# core: Interfaces

## Domain Types

### MailRequest (External API Contract)

```go
type MailRequest struct {
    AppTag          string            `json:"appTag" validate:"required"`
    Recipients      []string          `json:"recipients" validate:"required,min=1,dive,email"`
    CcRecipients    []string          `json:"ccRecipients,omitempty" validate:"dive,email"`
    BccRecipients   []string          `json:"bccRecipients,omitempty" validate:"dive,email"`
    Subject         string            `json:"subject,omitempty" validate:"max=998"`
    BodyContent     string            `json:"bodyContent,omitempty"`
    HtmlBodyContent string            `json:"htmlBodyContent,omitempty"`
    Attachments     []Attachment      `json:"attachments,omitempty"`
    TraceContext    map[string]string `json:"traceContext,omitempty"`
}

type Attachment struct {
    Name     string `json:"name"`
    MimeType string `json:"mimeType"`
    Content  string `json:"content"` // base64-encoded
}
```

### MailRequestDO (Internal NATS Representation)

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
    ObjectKey   string `json:"objectKey,omitempty"` // set by gateway
    Content     []byte `json:"-"`                   // set by worker, never serialized
}
```

### Sender

```go
type Sender struct {
    AppTag         string `json:"appTag"`
    Email          string `json:"email"`
    Test           bool   `json:"test"`
    DailyQuota     int    `json:"dailyQuota"`     // 0 or negative = unlimited
    AllowedDomains string `json:"allowedDomains"` // comma-separated, empty = all allowed
}
```

### Audit, DeadLetter, Bounce

```go
type AuditRecord struct {
    TraceID, AppTag, Status, Sender, Subject string
    Recipients []string
    Error      string
    Timestamp  time.Time
}

type DeadLetter struct {
    Payload, Error string
    Timestamp      time.Time
}

type BounceRecord struct {
    OriginalTraceID, BounceReason, BouncedRecipient string
    BouncedAt, ProcessedAt                          time.Time
}
```

## Error Types

```go
type ErrorCode string
const (
    ErrUnknownAppTag, ErrInvalidRecipientDomain, ErrQuotaExceeded,
    ErrSpamDetected, ErrInvalidAttachmentType, ErrAttachmentTooLarge,
    ErrBodyTooLarge, ErrGraphTimeout, ErrGraphServerError,
    ErrJSONParseError, ErrNatsUnavailable, ErrMessageTooLarge,
    ErrValidationFailed, ErrInternal
)

type ApiError struct { Status int; Code ErrorCode; Message, TraceID string }
type ValidationError struct { Code ErrorCode; Message string }
type QuotaError struct { Limit, Current, Requested int }
type QuotaStateError struct { Cause error }
type SpamStateError struct { Cause error }
type NatsPublishError struct { Cause error }
```

## Config

```go
type Config struct {
    Port                 string
    NatsURL              string
    MSGraphTenantID      string
    MSGraphClientID      string
    MSGraphClientSecret  string
    MSGraphSenderEmail   string
    MSGraphBounceMailbox string
    SpamTimeoutSeconds   int
    MaxBodySize          int64
    MimeWhitelist        []string
    MaxTotalAttachmentMB int
    NatsPublishTimeout   time.Duration
    GraphRateLimiterSkip bool
    GraphProxyURL        string
    GraphMockToken       string
    AdminAuthSecret      string
    GatewayAuthToken     string // DISPATCH_GATEWAY_AUTH_TOKEN (required by mail-gateway unless disabled)
    GatewayAuthDisabled  bool   // DISPATCH_GATEWAY_AUTH_DISABLED — local/dev only
}

func Load() (Config, error)
```

## Version

```go
var Version = "0.5.0" // set at build time via -ldflags
```
