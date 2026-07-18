package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

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

func (s *stubQuota) Check(_ string, _, _ int) error { return s.err }

type stubSpam struct{ err error }

func (s *stubSpam) Check(_ string) error { return s.err }

type stubPublisher struct{ err error }

func (s *stubPublisher) Publish(_ context.Context, _ *domain.MailRequestDO) error { return s.err }

type stubAttStore struct{}

func (s *stubAttStore) Upload(_ context.Context, _ string, atts []domain.Attachment) ([]domain.AttachmentDO, error) {
	result := make([]domain.AttachmentDO, len(atts))
	for i, a := range atts {
		result[i] = domain.AttachmentDO{Name: a.Name, ContentType: a.MimeType}
	}
	return result, nil
}

type failAttStore struct{}

func (s *failAttStore) Upload(_ context.Context, _ string, _ []domain.Attachment) ([]domain.AttachmentDO, error) {
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

func natsConnected() nats.Status { return nats.CONNECTED }

func buildHandler(senders senderLookup, quota quotaChecker, spam spamChecker, pub natsPublisher) *Handler {
	return NewHandler(defaultCfg(), senders, quota, spam, pub, &stubAttStore{}, natsConnected)
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

func TestHandleSend_InvalidRecipient_CodeIsValidationFailed(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{}, &stubSpam{}, &stubPublisher{},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{"notanemail"}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf(want400Fmt, rr.Code)
	}
	var apiErr domain.ApiError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatal(err)
	}
	if apiErr.Code != domain.ErrValidationFailed {
		t.Errorf("code: want VALIDATION_FAILED, got %q", apiErr.Code)
	}
}

func TestHandleSend_SpamStateError_Returns503(t *testing.T) {
	h := buildHandler(
		&stubSenders{sender: defaultSender()},
		&stubQuota{},
		&stubSpam{err: &domain.SpamStateError{Cause: errors.New("NATS down")}},
		&stubPublisher{},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestHandleSend_InternalError_CodeIsInternalError(t *testing.T) {
	h := buildHandler(
		&stubSenders{err: errors.New("kv connection lost")},
		&stubQuota{}, &stubSpam{}, &stubPublisher{},
	)
	body := map[string]any{"appTag": "test", "recipients": []string{testRecipient}}
	rr := sendRequest(t, h, body)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	var apiErr domain.ApiError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil {
		t.Fatal(err)
	}
	if apiErr.Code != domain.ErrInternal {
		t.Errorf("code: want INTERNAL_ERROR, got %q", apiErr.Code)
	}
	if apiErr.Message == "kv connection lost" {
		t.Error("internal error details must not leak to the client")
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
	h := NewHandler(defaultCfg(), &stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{}, &failAttStore{}, natsConnected)
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

func TestHandleSend_BodyTooLarge(t *testing.T) {
	// MaxBodySize=10: any body > 10 bytes triggers MaxBytesError.
	// Body must start with valid JSON chars so the scanner doesn't fail first.
	cfg := config.Config{MaxBodySize: 10, MimeWhitelist: []string{}, MaxTotalAttachmentMB: 20}
	h := NewHandler(cfg, &stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{}, &stubAttStore{}, natsConnected)
	body := `{"appTag":"test","bodyContent":"` + strings.Repeat("x", 100) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/dispatch/api/v1/mail/send", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestReady_NatsDown_Returns503(t *testing.T) {
	h := NewHandler(defaultCfg(), &stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{}, &stubAttStore{},
		func() nats.Status { return nats.DISCONNECTED })
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "DOWN") {
		t.Errorf("body must contain DOWN, got %s", rr.Body.String())
	}
}

func TestReady_NatsConnected_Returns200(t *testing.T) {
	h := NewHandler(defaultCfg(), &stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{}, &stubAttStore{}, natsConnected)
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "UP") {
		t.Errorf("body must contain UP, got %s", rr.Body.String())
	}
}

func TestLive_AlwaysReturns200(t *testing.T) {
	h := NewHandler(defaultCfg(), &stubSenders{sender: defaultSender()}, &stubQuota{}, &stubSpam{}, &stubPublisher{}, &stubAttStore{},
		func() nats.Status { return nats.DISCONNECTED })
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("liveness must stay 200, got %d", rr.Code)
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
