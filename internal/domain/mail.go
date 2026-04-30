package domain

type MailRequest struct {
	AppTag          string            `json:"appTag"          validate:"required"`
	Recipients      []string          `json:"recipients"      validate:"required,min=1,dive,email"`
	CcRecipients    []string          `json:"ccRecipients,omitempty"  validate:"dive,email"`
	BccRecipients   []string          `json:"bccRecipients,omitempty" validate:"dive,email"`
	Subject         string            `json:"subject,omitempty"       validate:"max=998"`
	BodyContent     string            `json:"bodyContent,omitempty"`
	HtmlBodyContent string            `json:"htmlBodyContent,omitempty"`
	Attachments     []Attachment      `json:"attachments,omitempty"`
	TraceContext    map[string]string `json:"traceContext,omitempty"`
}

type Attachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Content  string `json:"content"` // Base64-encoded
}
