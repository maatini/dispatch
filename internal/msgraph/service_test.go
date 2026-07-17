package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sony/gobreaker"

	"dispatch/internal/domain"
)

func testClient(httpClient *http.Client) *Client {
	return &Client{
		http:      httpClient,
		breaker:   gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test"}),
		mockToken: "test-token",
		tokens:    &tokenCache{},
	}
}

func testService(httpClient *http.Client, srvURL string) *Service {
	rl := NewRateLimiter(true)
	return &Service{
		client:      testClient(httpClient),
		rateLimiter: rl,
		baseURL:     srvURL,
	}
}

func chunkTestService(httpClient *http.Client) *Service {
	rl := NewRateLimiter(true)
	return &Service{
		client:      testClient(httpClient),
		rateLimiter: rl,
	}
}

// --- existing chunk tests ---

func TestUploadChunks_SingleChunk(t *testing.T) {
	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Content-Range")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	content := []byte("hello") // 5 bytes
	svc := chunkTestService(&http.Client{})
	if err := svc.uploadChunks(context.Background(), srv.URL, content); err != nil {
		t.Fatalf("uploadChunks: %v", err)
	}
	if gotRange != "bytes 0-4/5" {
		t.Errorf("Content-Range: want \"bytes 0-4/5\", got %q", gotRange)
	}
}

func TestUploadChunks_MultiChunk(t *testing.T) {
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Content-Range"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	total := chunkSize + 1
	content := make([]byte, total)
	svc := chunkTestService(&http.Client{})
	if err := svc.uploadChunks(context.Background(), srv.URL, content); err != nil {
		t.Fatalf("uploadChunks: %v", err)
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(ranges))
	}
	want := []string{
		fmt.Sprintf("bytes 0-%d/%d", chunkSize-1, total),
		fmt.Sprintf("bytes %d-%d/%d", chunkSize, chunkSize, total),
	}
	for i, w := range want {
		if ranges[i] != w {
			t.Errorf("chunk %d Content-Range: want %q, got %q", i, w, ranges[i])
		}
	}
}

func TestUploadChunks_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := chunkTestService(&http.Client{})
	err := svc.uploadChunks(context.Background(), srv.URL, []byte("hello"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Errorf("want GraphTransientError, got %T", err)
	}
}

func TestBuildGraphEmail_SetsTraceHeader(t *testing.T) {
	req := domain.MailRequestDO{
		TraceID:    "test-trace-id-123",
		Subject:    "Test",
		Recipients: []string{"to@example.com"},
	}
	result := buildGraphEmail(req, false)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed struct {
		Message struct {
			InternetMessageHeaders []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"internetMessageHeaders"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	headers := parsed.Message.InternetMessageHeaders
	if len(headers) != 1 {
		t.Fatalf("want 1 header, got %d", len(headers))
	}
	if headers[0].Name != "X-Dispatch-TraceId" {
		t.Errorf("header name: want X-Dispatch-TraceId, got %s", headers[0].Name)
	}
	if headers[0].Value != "test-trace-id-123" {
		t.Errorf("header value: want test-trace-id-123, got %s", headers[0].Value)
	}
}

// --- SendEmail ---

func TestSendEmail_SmallMessage_UsesInlinePath(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req := domain.MailRequestDO{
		Sender:     "s@example.com",
		Recipients: []string{"r@example.com"},
		Subject:    "Hi",
	}
	svc := testService(&http.Client{}, srv.URL)
	if err := svc.SendEmail(context.Background(), req); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if !strings.Contains(path, "sendMail") {
		t.Errorf("small message must use inline path, got %s", path)
	}
}

func TestSendEmail_LargeMessage_UsesUploadPath(t *testing.T) {
	var path string
	uploadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "messages") && !strings.Contains(r.URL.Path, "/send") && !strings.Contains(r.URL.Path, "createUploadSession") {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "draft-1"})
			return
		}
		if strings.Contains(r.URL.Path, "createUploadSession") {
			_ = json.NewEncoder(w).Encode(map[string]string{"uploadUrl": uploadSrv.URL + "/upload"})
			return
		}
		if strings.Contains(r.URL.Path, "/upload") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/send") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	largeContent := make([]byte, inlineThresholdBytes+1)
	req := domain.MailRequestDO{
		Sender:      "s@example.com",
		Recipients:  []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{{Name: "big.bin", Content: largeContent, ContentType: "application/octet-stream"}},
	}
	svc := testService(&http.Client{}, srv.URL)
	if err := svc.SendEmail(context.Background(), req); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if !strings.Contains(path, "/send") {
		t.Errorf("large message must use upload-session path, got %s", path)
	}
}

func TestSendEmail_RateLimiterError(t *testing.T) {
	rl := NewRateLimiter(false)
	for i := 0; i < 10; i++ {
		_ = rl.Wait(context.Background(), "s@example.com")
	}
	svc := &Service{
		client:      testClient(&http.Client{}),
		rateLimiter: rl,
	}

	ctx, cancel := context.WithDeadline(context.Background(), nowPlus(1))
	defer cancel()
	err := svc.SendEmail(ctx, domain.MailRequestDO{Sender: "s@example.com", Recipients: []string{"r@example.com"}})
	if err == nil {
		t.Fatal("expected rate limiter error")
	}
	if !strings.Contains(err.Error(), "rate limiter") {
		t.Errorf("error must mention rate limiter, got: %v", err)
	}
}

// --- sendViaUploadSession ---

func TestSendViaUploadSession_CreateDraftError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	largeContent := make([]byte, inlineThresholdBytes+1)
	req := domain.MailRequestDO{
		Sender:      "s@example.com",
		Recipients:  []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{{Name: "big.bin", Content: largeContent, ContentType: "application/octet-stream"}},
	}
	svc := testService(&http.Client{}, srv.URL)
	err := svc.SendEmail(context.Background(), req)
	if err == nil {
		t.Fatal("expected createDraft error")
	}
}

func TestSendViaUploadSession_SmallAttachmentUsesAddPath(t *testing.T) {
	addCalled := false
	uploadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/send") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(path, "createUploadSession") {
			_ = json.NewEncoder(w).Encode(map[string]string{"uploadUrl": uploadSrv.URL + "/upload"})
			return
		}
		if strings.Contains(path, "attachments") {
			addCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(path, "/messages") {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "draft-1"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	smallContent := []byte("small")
	largeContent := make([]byte, inlineThresholdBytes+1)
	req := domain.MailRequestDO{
		Sender:      "s@example.com",
		Recipients:  []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{{Name: "small.txt", Content: smallContent, ContentType: "text/plain"}, {Name: "big.bin", Content: largeContent, ContentType: "application/octet-stream"}},
	}
	svc := testService(&http.Client{}, srv.URL)
	if err := svc.SendEmail(context.Background(), req); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if !addCalled {
		t.Error("small attachment must use addSmallAttachment path")
	}
}

func TestSendViaUploadSession_AttachmentError_Cleanup(t *testing.T) {
	deleteCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/messages") && !strings.Contains(r.URL.Path, "/send") && !strings.Contains(r.URL.Path, "attachments") {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "draft-1"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	largeContent := make([]byte, inlineThresholdBytes+1)
	req := domain.MailRequestDO{
		Sender:      "s@example.com",
		Recipients:  []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{{Name: "big.bin", Content: largeContent, ContentType: "application/octet-stream"}},
	}
	svc := testService(&http.Client{}, srv.URL)
	err := svc.SendEmail(context.Background(), req)
	if err == nil {
		t.Fatal("expected attachment error")
	}
	if !deleteCalled {
		t.Error("attachment error must trigger cleanup (DELETE draft)")
	}
}

func TestSendViaUploadSession_FinalSendError_Cleanup(t *testing.T) {
	deleteCalled := false
	uploadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/messages") && !strings.Contains(r.URL.Path, "/send") && !strings.Contains(r.URL.Path, "attachments") {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "draft-1"})
			return
		}
		if strings.Contains(r.URL.Path, "createUploadSession") {
			_ = json.NewEncoder(w).Encode(map[string]string{"uploadUrl": uploadSrv.URL + "/upload"})
			return
		}
		if strings.Contains(r.URL.Path, "/upload") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/send") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	largeContent := make([]byte, inlineThresholdBytes+1)
	req := domain.MailRequestDO{
		Sender:      "s@example.com",
		Recipients:  []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{{Name: "big.bin", Content: largeContent, ContentType: "application/octet-stream"}},
	}
	svc := testService(&http.Client{}, srv.URL)
	err := svc.SendEmail(context.Background(), req)
	if err == nil {
		t.Fatal("expected final send error")
	}
	if !deleteCalled {
		t.Error("final send error must trigger cleanup")
	}
}

// --- createDraft ---

func TestCreateDraft_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	req := domain.MailRequestDO{Sender: "s@example.com", Recipients: []string{"r@example.com"}}
	svc := testService(&http.Client{}, srv.URL)
	_, err := svc.createDraft(context.Background(), req)
	if err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse draft response") {
		t.Errorf("error must mention parse draft response, got: %v", err)
	}
}

// --- uploadChunks (Bugfix 1.4) ---

func TestUploadChunks_400_IsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("expired url"))
	}))
	defer srv.Close()

	svc := chunkTestService(&http.Client{})
	err := svc.uploadChunks(context.Background(), srv.URL, []byte("hello"))
	var perm *GraphPermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("400 must return GraphPermanentError, got %T: %v", err, err)
	}
}

func TestUploadChunks_500_IsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := chunkTestService(&http.Client{})
	err := svc.uploadChunks(context.Background(), srv.URL, []byte("hello"))
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("500 must return GraphTransientError, got %T: %v", err, err)
	}
}

func TestUploadChunks_429_IsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	svc := chunkTestService(&http.Client{})
	err := svc.uploadChunks(context.Background(), srv.URL, []byte("hello"))
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("429 must return GraphTransientError, got %T: %v", err, err)
	}
}

func TestUploadChunks_NetworkError_IsTransient(t *testing.T) {
	svc := chunkTestService(&http.Client{})
	err := svc.uploadChunks(context.Background(), "http://127.0.0.1:0", []byte("hello"))
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("network error must be GraphTransientError, got %T: %v", err, err)
	}
}

// --- buildGraphEmail ---

func TestBuildGraphEmail_HtmlBody(t *testing.T) {
	req := domain.MailRequestDO{
		BodyContent:     "plain",
		HtmlBodyContent: "html",
		Recipients:      []string{"r@example.com"},
	}
	result := buildGraphEmail(req, false)
	if result.Message.Body.ContentType != "HTML" {
		t.Errorf("HTML body: want contentType HTML, got %s", result.Message.Body.ContentType)
	}
	if result.Message.Body.Content != "html" {
		t.Errorf("HTML body: want 'html', got %q", result.Message.Body.Content)
	}
}

func TestBuildGraphEmail_TextBody(t *testing.T) {
	req := domain.MailRequestDO{
		BodyContent: "plain",
		Recipients:  []string{"r@example.com"},
	}
	result := buildGraphEmail(req, false)
	if result.Message.Body.ContentType != "Text" {
		t.Errorf("text body: want contentType Text, got %s", result.Message.Body.ContentType)
	}
}

func TestBuildGraphEmail_CCAndBCCMapping(t *testing.T) {
	req := domain.MailRequestDO{
		CcRecipients:  []string{"cc@example.com"},
		BccRecipients: []string{"bcc@example.com"},
		Recipients:    []string{"to@example.com"},
	}
	result := buildGraphEmail(req, false)
	if len(result.Message.CcRecipients) != 1 {
		t.Errorf("CC: want 1, got %d", len(result.Message.CcRecipients))
	}
	if result.Message.CcRecipients[0].EmailAddress.Address != "cc@example.com" {
		t.Errorf("CC address mismatch")
	}
	if len(result.Message.BccRecipients) != 1 {
		t.Errorf("BCC: want 1, got %d", len(result.Message.BccRecipients))
	}
}

func TestBuildGraphEmail_IncludeAttachments(t *testing.T) {
	req := domain.MailRequestDO{
		Recipients: []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{
			{Name: "f.pdf", ContentType: "application/pdf", Content: []byte("data")},
		},
	}
	result := buildGraphEmail(req, true)
	if len(result.Message.Attachments) != 1 {
		t.Fatalf("attachments: want 1, got %d", len(result.Message.Attachments))
	}
	if result.Message.Attachments[0].Name != "f.pdf" {
		t.Errorf("attachment name mismatch")
	}
	if result.Message.Attachments[0].ODataType != "#microsoft.graph.fileAttachment" {
		t.Errorf("attachment ODataType mismatch")
	}
}

func TestBuildGraphEmail_ExcludeAttachments(t *testing.T) {
	req := domain.MailRequestDO{
		Recipients: []string{"r@example.com"},
		Attachments: []domain.AttachmentDO{
			{Name: "f.pdf", ContentType: "application/pdf"},
		},
	}
	result := buildGraphEmail(req, false)
	if len(result.Message.Attachments) != 0 {
		t.Errorf("attachments must be empty when includeAttachments=false")
	}
}

func nowPlus(d int) time.Time {
	return time.Now().Add(time.Duration(d) * time.Millisecond)
}
