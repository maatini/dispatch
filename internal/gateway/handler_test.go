package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"dispatch/internal/config"
	"dispatch/internal/domain"
)

const (
	testRecipient = "user@example.com"
	want400Fmt    = "expected 400, got %d"
)

// stubs

type stubSenders struct {
	sender domain.Sender
	err    error
}

func (s *stubSenders) Get(_ string) (domain.Sender, error) { return s.sender, s.err }

type stubQuota struct{ err error }

func (s *stubQuota) Check(_ string, _, _ int) error     { return s.err }
func (s *stubQuota) CurrentUsage(_ string) (int, error) { return 0, nil }

type stubSpam struct{ err error }

func (s *stubSpam) Check(_ string) error { return s.err }

type stubPublisher struct{ err error }

func (s *stubPublisher) Publish(_ context.Context, _ *domain.MailRequestDO) error { return s.err }

type stubAttStore struct{}

func (s *stubAttStore) Upload(_ context.Context, _ string, atts []domain.AttachmentDO) ([]domain.AttachmentDO, error) {
	return atts, nil
}

type failAttStore struct{}

func (s *failAttStore) Upload(_ context.Context, _ string, _ []domain.AttachmentDO) ([]domain.AttachmentDO, error) {
	return nil, errors.New("object store unavailable")
}

func defaultCfg() config.Config {
	return config.Config{
		MaxBodySize:          10_000_000,
		MimeWhitelist:        []string{"application/pdf", "image/jpeg"},
		MaxTotalAttachmentMB: 20,
	}
}

func defaultSender() domain.Sender {
	return domain.Sender{AppTag: "test", Email: "noreply@example.com", DailyQuota: 100}
}

func buildHandler(senders senderLookup, quota quotaChecker, spam spamChecker, pub natsPublisher) *Handler {
	return NewHandler(defaultCfg(), senders, quota, spam, pub, &stubAttStore{})
}

func sendRequest(t *testing.T, h *Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/dispatch/api/v1/mail/send", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	return rr
}

func TestHandleSend_Success(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{},
		&stubSpam{},
		&stubPublisher{},
	)
	body := map[string]any{
		"appTag":     "test",
		"recipients": []string{testRecipient},
		"subject":    "Hello",
	}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleSend_MissingAppTag(t *testing.T) {
	h := buildHandler(&stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{})
	body := map[string]any{"recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf(want400Fmt, rr.Code)
	}
}

func TestHandleSend_UnknownAppTag(t *testing.T) {
	h := buildHandler(
		&stubSenders{err: &domain.ValidationError{Code: domain.ErrUnknownAppTag, Message: "unknown"}},
		&stubQuota{}, &stubSpam{}, &stubPublisher{},
	)
	body := map[string]any{"appTag": "unknown", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf(want400Fmt, rr.Code)
	}
}

func TestHandleSend_QuotaExceeded(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{err: &domain.QuotaError{Limit: 10, Current: 10, Requested: 1}},
		&stubSpam{}, &stubPublisher{},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	// defaultSender().DailyQuota=100, qe.Current=10 → Remaining=max(0,100-10)=90
	if got := rr.Header().Get("X-RateLimit-Limit"); got != "100" {
		t.Errorf("X-RateLimit-Limit: want 100, got %s", got)
	}
	if got := rr.Header().Get("X-RateLimit-Remaining"); got != "90" {
		t.Errorf("X-RateLimit-Remaining: want 90, got %s", got)
	}
}

func TestHandleSend_SpamDetected(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{},
		&stubSpam{err: &domain.ValidationError{Code: domain.ErrSpamDetected, Message: "dup"}},
		&stubPublisher{},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf(want400Fmt, rr.Code)
	}
}

func TestHandleSend_NatsUnavailable(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{},
		&stubSpam{},
		&stubPublisher{err: &domain.NatsPublishError{Cause: errors.New("connection refused")}},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestHandleSend_QuotaStateError(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{err: &domain.QuotaStateError{Cause: errors.New("NATS down")}},
		&stubSpam{}, &stubPublisher{},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestHandleSend_AttachmentUploadError(t *testing.T) {
	h := NewHandler(defaultCfg(), &stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{}, &failAttStore{})
	body := map[string]any{
		"appTag":     "test",
		"recipients": []string{testRecipient},
		"subject":    "Test",
		"attachments": []map[string]any{
			{"name": "file.pdf", "mimeType": "application/pdf", "content": "dGVzdA=="},
		},
	}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleHealth(t *testing.T) {
	h := buildHandler(&stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}
}

func TestHandleSend_WithCCAndBCC(t *testing.T) {
	h := buildHandler(&stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{})
	body := map[string]any{
		"appTag":        "test",
		"recipients":    []string{"to@example.com"},
		"ccRecipients":  []string{"cc@example.com"},
		"bccRecipients": []string{"bcc@example.com"},
		"subject":       "Hello",
	}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}
