package gateway

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/go-playground/validator/v10"

	"dispatch/internal/domain"
)

var validate = validator.New()

func validateRequest(req *domain.MailRequest, maxBodySize int64, mimeWhitelist []string, maxAttachMB int) error {
	if err := validate.Struct(req); err != nil {
		return &domain.ValidationError{
			Code:    domain.ErrUnknownAppTag,
			Message: fmt.Sprintf("request validation failed: %v", err),
		}
	}

	bodySize := int64(len(req.BodyContent)) + int64(len(req.HtmlBodyContent))
	if bodySize > maxBodySize {
		return &domain.ValidationError{
			Code:    domain.ErrBodyTooLarge,
			Message: fmt.Sprintf("body size %d exceeds limit %d", bodySize, maxBodySize),
		}
	}

	if len(req.Attachments) > 0 {
		whitelistSet := make(map[string]bool, len(mimeWhitelist))
		for _, m := range mimeWhitelist {
			whitelistSet[strings.TrimSpace(m)] = true
		}

		var totalBytes int64
		for _, a := range req.Attachments {
			if !whitelistSet[a.MimeType] {
				return &domain.ValidationError{
					Code:    domain.ErrInvalidAttachmentType,
					Message: fmt.Sprintf("MIME type not allowed: %s", a.MimeType),
				}
			}
			r := base64.NewDecoder(base64.StdEncoding, strings.NewReader(a.Content))
			n, ioErr := io.Copy(io.Discard, r)
			if ioErr != nil {
				return &domain.ValidationError{
					Code:    domain.ErrInvalidAttachmentType,
					Message: fmt.Sprintf("attachment %q: invalid base64", a.Name),
				}
			}
			totalBytes += n
		}

		maxBytes := int64(maxAttachMB) * 1024 * 1024
		if totalBytes > maxBytes {
			return &domain.ValidationError{
				Code:    domain.ErrAttachmentTooLarge,
				Message: fmt.Sprintf("total attachment size %d bytes exceeds limit %d MB", totalBytes, maxAttachMB),
			}
		}
	}

	return nil
}

func checkDomains(sender domain.Sender, req *domain.MailRequest) error {
	if sender.AllowedDomains == "" {
		return nil
	}
	allowed := make(map[string]bool)
	for _, d := range strings.Split(sender.AllowedDomains, ",") {
		allowed[strings.TrimSpace(strings.ToLower(d))] = true
	}

	allRecipients := append(append(req.Recipients, req.CcRecipients...), req.BccRecipients...)
	for _, addr := range allRecipients {
		parts := strings.SplitN(addr, "@", 2)
		if len(parts) != 2 {
			return &domain.ValidationError{
				Code:    domain.ErrInvalidRecipientDomain,
				Message: fmt.Sprintf("invalid recipient address: %s", addr),
			}
		}
		recipDomain := strings.ToLower(parts[1])
		if !allowed[recipDomain] {
			return &domain.ValidationError{
				Code:    domain.ErrInvalidRecipientDomain,
				Message: fmt.Sprintf("recipient domain not allowed: %s", recipDomain),
			}
		}
	}
	return nil
}
