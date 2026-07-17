package msgraph

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sony/gobreaker"
)

const (
	testBounceMailbox = "bounce@example.com"
	errUnexpected     = "unexpected error: %v"
)

func testBounceService(srv *httptest.Server) *BounceService {
	return &BounceService{
		client: &Client{
			http:      srv.Client(),
			breaker:   gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test"}),
			tokens:    &tokenCache{},
			mockToken: "test-token",
		},
		baseURL: srv.URL,
	}
}

func TestBounceService_GetUnreadMessages(t *testing.T) {
	var gotPath, gotFilter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFilter = r.URL.Query().Get("$filter")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[
			{"id":"msg1","subject":"Undeliverable: Hello","body":{"content":"X-Dispatch-TraceId: abc-123"},"toRecipients":[{"emailAddress":{"address":"bounced@example.com"}}],"receivedDateTime":"2026-01-15T10:30:00Z"},
			{"id":"msg2","subject":"NDR","body":{"content":"no trace id"},"toRecipients":[],"receivedDateTime":""}
		]}`))
	}))
	defer srv.Close()

	svc := testBounceService(srv)
	msgs, err := svc.GetUnreadMessages(context.Background(), testBounceMailbox)
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if gotPath != "/users/bounce@example.com/messages" {
		t.Errorf("path: want /users/bounce@example.com/messages, got %s", gotPath)
	}
	if gotFilter != "isRead eq false" {
		t.Errorf("$filter: want 'isRead eq false', got %q", gotFilter)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].ID != "msg1" || msgs[0].Subject != "Undeliverable: Hello" || msgs[0].Body != "X-Dispatch-TraceId: abc-123" {
		t.Errorf("message[0]: got %+v", msgs[0])
	}
	if msgs[0].Recipient != "bounced@example.com" {
		t.Errorf("message[0].Recipient: want bounced@example.com, got %s", msgs[0].Recipient)
	}
	if msgs[0].ReceivedAt.IsZero() {
		t.Error("message[0].ReceivedAt should not be zero")
	}
	if msgs[1].ID != "msg2" {
		t.Errorf("message[1].ID: want msg2, got %s", msgs[1].ID)
	}
}

func TestBounceService_GetUnreadMessages_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer srv.Close()

	svc := testBounceService(srv)
	msgs, err := svc.GetUnreadMessages(context.Background(), testBounceMailbox)
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestBounceService_GetUnreadMessages_GraphError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 4xx → GraphPermanentError, no retry
	}))
	defer srv.Close()

	svc := testBounceService(srv)
	_, err := svc.GetUnreadMessages(context.Background(), testBounceMailbox)
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
}

func TestBounceService_MarkAsRead(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := testBounceService(srv)
	if err := svc.MarkAsRead(context.Background(), testBounceMailbox, "msg-id-123"); err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method: want PATCH, got %s", gotMethod)
	}
	if gotPath != "/users/bounce@example.com/messages/msg-id-123" {
		t.Errorf("path: want .../messages/msg-id-123, got %s", gotPath)
	}
	if !bytes.Contains(gotBody, []byte(`"isRead":true`)) {
		t.Errorf("body should contain isRead:true, got %s", gotBody)
	}
}

func TestBounceService_MarkAsRead_GraphError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 4xx → GraphPermanentError, no retry
	}))
	defer srv.Close()

	svc := testBounceService(srv)
	err := svc.MarkAsRead(context.Background(), testBounceMailbox, "gone")
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestParseNDRMessages_Valid(t *testing.T) {
	data := []byte(`{"value":[{"id":"x","subject":"NDR","body":{"content":"trace body"},"toRecipients":[{"emailAddress":{"address":"b@x.com"}}],"receivedDateTime":"2026-01-15T10:30:00Z"}]}`)
	msgs, err := parseNDRMessages(data)
	if err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(msgs) != 1 || msgs[0].ID != "x" || msgs[0].Subject != "NDR" || msgs[0].Body != "trace body" {
		t.Errorf("unexpected result: %+v", msgs)
	}
	if msgs[0].Recipient != "b@x.com" {
		t.Errorf("Recipient: want b@x.com, got %s", msgs[0].Recipient)
	}
	if msgs[0].ReceivedAt.IsZero() {
		t.Error("ReceivedAt should not be zero")
	}
}

func TestParseNDRMessages_InvalidJSON(t *testing.T) {
	_, err := parseNDRMessages([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
