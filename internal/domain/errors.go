package domain

import "fmt"

type ErrorCode string

const (
	ErrUnknownAppTag          ErrorCode = "UNKNOWN_APP_TAG"
	ErrInvalidRecipientDomain ErrorCode = "INVALID_RECIPIENT_DOMAIN"
	ErrQuotaExceeded          ErrorCode = "QUOTA_EXCEEDED"
	ErrSpamDetected           ErrorCode = "SPAM_DETECTED"
	ErrInvalidAttachmentType  ErrorCode = "INVALID_ATTACHMENT_TYPE"
	ErrAttachmentTooLarge     ErrorCode = "ATTACHMENT_TOO_LARGE"
	ErrBodyTooLarge           ErrorCode = "BODY_TOO_LARGE"
	ErrGraphTimeout           ErrorCode = "GRAPH_TIMEOUT"
	ErrGraphServerError       ErrorCode = "GRAPH_SERVER_ERROR"
	ErrJSONParseError         ErrorCode = "JSON_PARSE_ERROR"
	ErrNatsUnavailable        ErrorCode = "NATS_UNAVAILABLE"
	ErrMessageTooLarge        ErrorCode = "MESSAGE_TOO_LARGE"
)

type ApiError struct {
	Status  int       `json:"status"`
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	TraceID string    `json:"traceId,omitempty"`
}

type ValidationError struct {
	Code    ErrorCode
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type QuotaError struct {
	Limit     int
	Current   int
	Requested int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("quota exceeded: limit=%d current=%d requested=%d", e.Limit, e.Current, e.Requested)
}

type QuotaStateError struct {
	Cause error
}

func (e *QuotaStateError) Error() string {
	return fmt.Sprintf("quota state unavailable: %v", e.Cause)
}

func (e *QuotaStateError) Unwrap() error { return e.Cause }

type NatsPublishError struct {
	Cause error
}

func (e *NatsPublishError) Error() string {
	return fmt.Sprintf("NATS publish failed: %v", e.Cause)
}

func (e *NatsPublishError) Unwrap() error { return e.Cause }
