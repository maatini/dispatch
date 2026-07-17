package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dispatch/internal/domain"
)

func chunkTestService(httpClient *http.Client) *Service {
	return &Service{client: &Client{http: httpClient}}
}

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
	// bytes 0-4/5: end=5, end-1=4 (not end+1=6)
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
