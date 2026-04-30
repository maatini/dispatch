package gateway

import (
	"errors"
	"strings"
	"testing"

	"codymail-go/internal/domain"
)

func TestValidateRequest_Valid(t *testing.T) {
	req := &domain.MailRequest{
		AppTag:     "test",
		Recipients: []string{"a@b.com"},
		Subject:    "Hello",
	}
	if err := validateRequest(req, 10_000_000, []string{"application/pdf"}, 20); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateRequest_MissingAppTag(t *testing.T) {
	req := &domain.MailRequest{Recipients: []string{"a@b.com"}}
	err := validateRequest(req, 10_000_000, []string{"application/pdf"}, 20)
	if err == nil {
		t.Fatal("expected error for missing appTag")
	}
}

func TestValidateRequest_InvalidEmail(t *testing.T) {
	req := &domain.MailRequest{AppTag: "t", Recipients: []string{"notanemail"}}
	err := validateRequest(req, 10_000_000, []string{"application/pdf"}, 20)
	if err == nil {
		t.Fatal("expected error for invalid email")
	}
}

func TestValidateRequest_SubjectTooLong(t *testing.T) {
	req := &domain.MailRequest{
		AppTag:     "t",
		Recipients: []string{"a@b.com"},
		Subject:    strings.Repeat("x", 999),
	}
	err := validateRequest(req, 10_000_000, []string{"application/pdf"}, 20)
	if err == nil {
		t.Fatal("expected error for subject too long")
	}
}

func TestValidateRequest_BodyTooLarge(t *testing.T) {
	req := &domain.MailRequest{
		AppTag:      "t",
		Recipients:  []string{"a@b.com"},
		BodyContent: strings.Repeat("x", 100),
	}
	err := validateRequest(req, 50, []string{}, 20)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Code != domain.ErrBodyTooLarge {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}

func TestValidateRequest_InvalidMimeType(t *testing.T) {
	req := &domain.MailRequest{
		AppTag:     "t",
		Recipients: []string{"a@b.com"},
		Attachments: []domain.Attachment{
			{Name: "file.exe", MimeType: "application/octet-stream", Content: "AA=="},
		},
	}
	err := validateRequest(req, 10_000_000, []string{"application/pdf"}, 20)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Code != domain.ErrInvalidAttachmentType {
		t.Fatalf("expected ErrInvalidAttachmentType, got %v", err)
	}
}

func TestCheckDomains_AllowedAll(t *testing.T) {
	sender := domain.Sender{AllowedDomains: ""}
	req := &domain.MailRequest{Recipients: []string{"user@anything.com"}}
	if err := checkDomains(sender, req); err != nil {
		t.Fatalf("empty allowed domains should allow all: %v", err)
	}
}

func TestCheckDomains_Allowed(t *testing.T) {
	sender := domain.Sender{AllowedDomains: "example.com, test.de"}
	req := &domain.MailRequest{Recipients: []string{"user@example.com"}}
	if err := checkDomains(sender, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckDomains_Blocked(t *testing.T) {
	sender := domain.Sender{AllowedDomains: "example.com"}
	req := &domain.MailRequest{Recipients: []string{"user@other.com"}}
	err := checkDomains(sender, req)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Code != domain.ErrInvalidRecipientDomain {
		t.Fatalf("expected ErrInvalidRecipientDomain, got %v", err)
	}
}

func TestCheckDomains_CCBlocked(t *testing.T) {
	sender := domain.Sender{AllowedDomains: "example.com"}
	req := &domain.MailRequest{
		Recipients:   []string{"user@example.com"},
		CcRecipients: []string{"cc@blocked.com"},
	}
	err := checkDomains(sender, req)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Code != domain.ErrInvalidRecipientDomain {
		t.Fatalf("expected ErrInvalidRecipientDomain for CC, got %v", err)
	}
}
