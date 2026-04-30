package domain

import "time"

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
	Content     []byte `json:"contentBytes"`
}

type AuditRecord struct {
	TraceID    string    `json:"traceId"`
	AppTag     string    `json:"appTag"`
	Status     string    `json:"status"` // DELIVERED, FAILED, TEST_SUCCESS
	Sender     string    `json:"sender"`
	Subject    string    `json:"subject"`
	Recipients []string  `json:"recipients"`
	Error      string    `json:"error,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

type DeadLetter struct {
	Payload   string    `json:"payload"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

type BounceRecord struct {
	OriginalTraceID  string    `json:"originalTraceId"`
	BouncedAt        time.Time `json:"bouncedAt"`
	BounceReason     string    `json:"bounceReason"`
	BouncedRecipient string    `json:"bouncedRecipient"`
	ProcessedAt      time.Time `json:"processedAt"`
}

const (
	StatusDelivered   = "DELIVERED"
	StatusFailed      = "FAILED"
	StatusTestSuccess = "TEST_SUCCESS"
)
