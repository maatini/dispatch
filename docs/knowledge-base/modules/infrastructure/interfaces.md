# infrastructure: Interfaces

## loggy

### Logger Creation
```go
func GetLogger(className string) *Loggy
func (l *Loggy) With(attrs ...slog.Attr) *Loggy
```

### Field Helpers
```go
func Kv(key string, value any) slog.Attr
```

### Standard Log Methods
```go
func (l *Loggy) Info(msg string, fields ...slog.Attr)
func (l *Loggy) Warn(msg string, fields ...slog.Attr)
func (l *Loggy) Error(msg string, err error, fields ...slog.Attr)
```

### Category-Variant Methods (with context)
```go
func (l *Loggy) Infoc(ctx context.Context, category LogCategory, msg string, fields ...slog.Attr)
func (l *Loggy) Warnc(ctx context.Context, category LogCategory, msg string, fields ...slog.Attr)
func (l *Loggy) Errorc(ctx context.Context, category LogCategory, msg string, err error, fields ...slog.Attr)
```

### Semantic Methods
```go
func (l *Loggy) Critical(msg string, err error, fields ...slog.Attr)
```

### API Tracking
```go
func (l *Loggy) RecordApiStart(apiName string)
func (l *Loggy) ExternalApiSuccess(apiName string, httpStatus int)
func (l *Loggy) ExternalApiFailure(apiName string, httpStatus int, err error)
func (l *Loggy) ApiClientError(apiName string, httpStatus int, reason string)
```

### Log Categories
```go
CategoryCritical, CategoryBusinessLogic, CategoryBusinessRuleViolation,
CategoryAPIRequest, CategoryAPIExternalFailure, CategoryAPIClientError,
CategoryInfo, CategoryDefault
```

## natsutil

### Connection & Provisioning
```go
func Connect(url string) (*nats.Conn, nats.JetStreamContext, error)
// Setup = ProvisionStreams + ProvisionKVBuckets (used by all cmd/*/main.go)
func Setup(js nats.JetStreamContext, spamTTL time.Duration) error
func ProvisionStreams(js nats.JetStreamContext) error
func ProvisionKVBuckets(js nats.JetStreamContext, spamTTL time.Duration) error
func ProvisionObjectStore(js nats.JetStreamContext) (nats.ObjectStore, error)
func ProvisionWorkerConsumer(js nats.JetStreamContext) error
```

### Name Constants
```go
// Streams
StreamMails      = "DISPATCH_MAILS"
StreamAudit      = "DISPATCH_AUDIT"
StreamDeadLetter = "DISPATCH_DEAD_LETTERS"
StreamBounces    = "DISPATCH_BOUNCES"

// Subjects
SubjectMails      = "cody.mailing.job.request.mails"
SubjectAudit      = "cody.mailing.audit"
SubjectDeadLetter = "cody.mailing.deadletter"
SubjectBounce     = "cody.mailing.bounce"

// KV Buckets
BucketSenders     = "senders"
BucketQuota       = "quota"
BucketSpam        = "spam"
BucketDelivered   = "delivered"
BucketAttachments = "attachments"

// Consumers
ConsumerMailWorker = "mail-worker"
```

## httpsrv

Shared HTTP server lifecycle (used by `cmd/mail-gateway` and `cmd/mail-admin`).

```go
// Run serves HTTP on addr until ctx is cancelled, then shuts down gracefully
// (10s shutdown timeout). Listen/serve failures are returned; shutdown errors are logged.
func Run(ctx context.Context, name, addr string, handler http.Handler) error
```

## testkit

Shared in-memory NATS KV mock for unit tests (not used in production binaries).

```go
type MockKV struct {
    Data, Revisions map[...]
    GetErr, PutErr, CreateErr, UpdateErr, DeleteErr, KeysErr error
    KeysList []string
}

func NewMockKV() *MockKV
// Implements Get/Put/Create/Update/Delete/Keys with optional error injection and CAS via Revisions.
type WrongSeqError struct{} // implements nats.JetStreamError for CAS-conflict tests
```

## hash

```go
func SpamHash(appTag, subject string, recipients []string, bodyLen, htmlLen int) string
```

## pii

```go
func MaskEmail(email string) string
// "user@domain.com" → "u***@domain.com"
```
