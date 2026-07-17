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
func Alert(v bool) slog.Attr
```

### Standard Log Methods
```go
func (l *Loggy) Info(msg string, fields ...slog.Attr)
func (l *Loggy) Warn(msg string, fields ...slog.Attr)
func (l *Loggy) Error(msg string, err error, fields ...slog.Attr)
func (l *Loggy) Debug(msg string, fields ...slog.Attr)
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
func (l *Loggy) BusinessRuleViolation(ruleID, msg, ctx string)
func (l *Loggy) ValidationFailed(field, reason string, value ...string)
func (l *Loggy) MissingData(field, ctx string)
func (l *Loggy) UncaughtException(err error, ctx string)
func (l *Loggy) ServiceAccountExpired(accountID, ctx string)
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
CategoryMissingData, CategoryValidation, CategoryAPIRequest,
CategoryAPIExternalFailure, CategoryAPIClientError, CategoryUncaughtException,
CategorySecurity, CategoryPerformance, CategoryInfo, CategoryUnstructured
```

## natsutil

### Connection & Provisioning
```go
func Connect(url string) (*nats.Conn, nats.JetStreamContext, error)
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

## hash

```go
func SpamHash(appTag, subject string, recipients []string, bodyLen, htmlLen int) string
```

## pii

```go
func MaskEmail(email string) string
// "user@domain.com" → "u***@domain.com"
```
