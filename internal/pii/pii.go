package pii

import "strings"

// MaskEmail masks an email address for safe logging: "user@domain.com" → "u***@domain.com"
func MaskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) == 1 {
		return local + "***" + domain
	}
	return string(local[0]) + "***" + domain
}
