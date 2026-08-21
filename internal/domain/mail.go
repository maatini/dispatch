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

const (
	traceKeyParent = "traceparent"
	traceKeyState  = "tracestate"
	maxTraceState  = 256
)

// SanitizeTraceContext keeps only W3C traceparent/tracestate with valid values.
// Unknown keys (and invalid values) are dropped so callers cannot inject headers.
func SanitizeTraceContext(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, 2)
	for k, v := range in {
		switch lowerASCII(k) {
		case traceKeyParent:
			if validTraceparent(v) {
				out[traceKeyParent] = v
			}
		case traceKeyState:
			if validTracestate(v) {
				out[traceKeyState] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// validTraceparent accepts W3C version-format: 2-32-16-2 lowercase hex (55 chars).
func validTraceparent(s string) bool {
	if len(s) != 55 || s[2] != '-' || s[35] != '-' || s[52] != '-' {
		return false
	}
	return isHexLower(s[0:2]) && isHexLower(s[3:35]) && isHexLower(s[36:52]) && isHexLower(s[53:55])
}

func validTracestate(s string) bool {
	if s == "" || len(s) > maxTraceState {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func isHexLower(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
