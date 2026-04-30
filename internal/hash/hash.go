package hash

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// SpamHash computes a SHA-256 fingerprint for spam deduplication.
// Inputs: appTag, subject, sorted recipients, body lengths.
func SpamHash(appTag, subject string, recipients []string, bodyLen, htmlLen int) string {
	input := strings.Join([]string{
		appTag,
		subject,
		strings.Join(recipients, ","),
		fmt.Sprintf("%d", bodyLen),
		fmt.Sprintf("%d", htmlLen),
	}, "|")
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum)
}
