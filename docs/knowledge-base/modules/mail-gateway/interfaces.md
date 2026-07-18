# mail-gateway: Interfaces

## HTTP Endpoints

### `POST /dispatch/api/v1/mail/send`

**Request:**
```json
{
  "appTag": "string (required)",
  "recipients": ["email (required, min 1)"],
  "ccRecipients": ["email (optional)"],
  "bccRecipients": ["email (optional)"],
  "subject": "string (optional, max 998)",
  "bodyContent": "string (optional)",
  "htmlBodyContent": "string (optional)",
  "attachments": [{
    "name": "string",
    "mimeType": "string (whitelist-checked)",
    "content": "base64-encoded string"
  }],
  "traceContext": {}
}
```

**Success Response (202):**
```json
{"status": "SUCCESS", "traceId": "uuid"}
```

**Error Response (400/413/429/503):**
```json
{
  "status": 400,
  "code": "ERROR_CODE",
  "message": "human-readable description",
  "traceId": "uuid"
}
```

### `GET /health`
```json
{"status": "UP", "checks": [{"name": "nats", "status": "UP"}]}
```
200 when the NATS connection is `CONNECTED`, otherwise 503 with `"status": "DOWN"`.

### `GET /health/live`
200 OK (empty body) — always, pure liveness

### `GET /health/ready`
Same logic as `/health`: 200 UP / 503 DOWN based on real NATS connectivity

## Error Codes

| HTTP | Code | Trigger |
|---|---|---|
| 400 | `UNKNOWN_APP_TAG` | appTag not found in sender KV |
| 400 | `VALIDATION_FAILED` | Struct validation failure (missing/invalid fields) |
| 400 | `INVALID_RECIPIENT_DOMAIN` | Domain not in sender's allowed list |
| 400 | `SPAM_DETECTED` | Duplicate message within spam window |
| 400 | `INVALID_ATTACHMENT_TYPE` | MIME type not whitelisted or invalid base64 |
| 400 | `ATTACHMENT_TOO_LARGE` | Total attachment size exceeds limit |
| 400 | `BODY_TOO_LARGE` | Request body exceeds MaxBytesReader limit |
| 400 | `JSON_PARSE_ERROR` | Invalid JSON body |
| 413 | `BODY_TOO_LARGE` | Body exceeds `DISPATCH_VALIDATION_MAX_BODY_SIZE` |
| 429 | `QUOTA_EXCEEDED` | Daily recipient quota reached (+ `X-RateLimit-*` headers) |
| 500 | `INTERNAL_ERROR` | Unexpected non-domain error (generic message, no internal details) |
| 503 | `NATS_UNAVAILABLE` | NATS publish failure, quota/spam KV error, or attachment upload failure |

## Interface Contracts (consumer-side)

```go
type senderLookup interface {
    Get(appTag string) (domain.Sender, error)
}

type quotaChecker interface {
    Check(appTag string, limit, requested int) error
}

type spamChecker interface {
    Check(hashVal string) error
}

type natsPublisher interface {
    Publish(ctx context.Context, msg *domain.MailRequestDO) error
}

type attachmentUploader interface {
    Upload(ctx context.Context, traceID string, attachments []domain.Attachment) ([]domain.AttachmentDO, error)
}
```

## Rate Limit Headers

When quota is exceeded (429), the response includes:
- `X-RateLimit-Limit: <dailyQuota>`
- `X-RateLimit-Remaining: max(0, limit - current)`
