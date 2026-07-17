package bounce

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

const (
	testCrawlerMailbox = "bounce@example.com"
	errUnexpected      = "unexpected error: %v"
)

// --- stubs ---

type stubGraph struct {
	msgs       []NDRMessage
	err        error
	readCalled []string // message IDs for which MarkAsRead was called
	readErr    error
}

func (s *stubGraph) GetUnreadMessages(_ context.Context, _ string) ([]NDRMessage, error) {
	return s.msgs, s.err
}
func (s *stubGraph) MarkAsRead(_ context.Context, _, messageID string) error {
	s.readCalled = append(s.readCalled, messageID)
	return s.readErr
}

type captureJS struct {
	published [][]byte
	err       error
}

func (c *captureJS) Publish(_ string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	if c.err != nil {
		return nil, c.err
	}
	c.published = append(c.published, data)
	return &nats.PubAck{}, nil
}

// --- extractTraceID ---

func TestExtractTraceID_Found(t *testing.T) {
	body := "Some NDR text\nX-Dispatch-TraceId: 550e8400-e29b-41d4-a716-446655440000\nMore text"
	got := extractTraceID(body)
	if got != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("want trace ID, got %q", got)
	}
}

func TestExtractTraceID_NotFound(t *testing.T) {
	got := extractTraceID("no trace id here")
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestExtractTraceID_Empty(t *testing.T) {
	got := extractTraceID("")
	if got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

// --- Crawler.Run ---

func TestRun_NoMessages(t *testing.T) {
	crawler := NewCrawler(&stubGraph{msgs: []NDRMessage{}}, &captureJS{}, testCrawlerMailbox)
	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

func TestRun_GraphError(t *testing.T) {
	crawler := NewCrawler(&stubGraph{err: errors.New("graph down")}, &captureJS{}, testCrawlerMailbox)
	if err := crawler.Run(context.Background()); err == nil {
		t.Fatal("expected error when graph fails")
	}
}

func TestRun_PublishesBouncRecord(t *testing.T) {
	msgs := []NDRMessage{
		{ID: "m1", Subject: "Undeliverable: Hi", Body: "X-Dispatch-TraceId: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
	}
	js := &captureJS{}
	crawler := NewCrawler(&stubGraph{msgs: msgs}, js, testCrawlerMailbox)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(js.published) != 1 {
		t.Fatalf("want 1 published message, got %d", len(js.published))
	}

	var rec domain.BounceRecord
	if err := json.Unmarshal(js.published[0], &rec); err != nil {
		t.Fatalf("unmarshal bounce record: %v", err)
	}
	if rec.OriginalTraceID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("OriginalTraceID: want trace id, got %q", rec.OriginalTraceID)
	}
	if rec.BounceReason != "Undeliverable: Hi" {
		t.Errorf("BounceReason: want subject, got %q", rec.BounceReason)
	}
}

func TestRun_NoTraceID_StillPublishes(t *testing.T) {
	msgs := []NDRMessage{
		{ID: "m2", Subject: "NDR", Body: "no trace id in body"},
	}
	js := &captureJS{}
	crawler := NewCrawler(&stubGraph{msgs: msgs}, js, testCrawlerMailbox)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(js.published) != 1 {
		t.Fatalf("want 1 published message, got %d", len(js.published))
	}

	var rec domain.BounceRecord
	_ = json.Unmarshal(js.published[0], &rec)
	if rec.OriginalTraceID != "" {
		t.Errorf("OriginalTraceID: want empty, got %q", rec.OriginalTraceID)
	}
}

func TestRun_PublishError_ContinuesToNextMessage(t *testing.T) {
	msgs := []NDRMessage{
		{ID: "m1", Subject: "NDR1", Body: ""},
		{ID: "m2", Subject: "NDR2", Body: ""},
	}
	js := &captureJS{err: errors.New("NATS down")}
	crawler := NewCrawler(&stubGraph{msgs: msgs}, js, testCrawlerMailbox)

	// Run must not return an error — publish errors are logged but do not abort the loop
	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
}

func TestRun_MultipleMessages(t *testing.T) {
	msgs := []NDRMessage{
		{ID: "m1", Subject: "NDR1", Body: "X-Dispatch-TraceId: 11111111-1111-1111-1111-111111111111"},
		{ID: "m2", Subject: "NDR2", Body: "X-Dispatch-TraceId: 22222222-2222-2222-2222-222222222222"},
	}
	js := &captureJS{}
	graph := &stubGraph{msgs: msgs}
	crawler := NewCrawler(graph, js, testCrawlerMailbox)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(js.published) != 2 {
		t.Errorf("want 2 published messages, got %d", len(js.published))
	}
	if len(graph.readCalled) != 2 {
		t.Errorf("want 2 MarkAsRead calls, got %d", len(graph.readCalled))
	}
}

func TestRun_PublishError_SkipsMarkAsRead(t *testing.T) {
	msgs := []NDRMessage{
		{ID: "m1", Subject: "NDR1", Body: ""},
	}
	js := &captureJS{err: errors.New("NATS down")}
	graph := &stubGraph{msgs: msgs}
	crawler := NewCrawler(graph, js, testCrawlerMailbox)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
	if len(graph.readCalled) != 0 {
		t.Errorf("MarkAsRead must not be called when publish fails, got %d calls", len(graph.readCalled))
	}
}

func TestProcess_UsesReceivedAtFromNDR(t *testing.T) {
	now := mustParseTime("2026-01-15T10:30:00Z")
	msgs := []NDRMessage{
		{ID: "m1", Subject: "NDR", Body: "", Recipient: "bounced@example.com", ReceivedAt: now},
	}
	js := &captureJS{}
	crawler := NewCrawler(&stubGraph{msgs: msgs}, js, testCrawlerMailbox)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf(errUnexpected, err)
	}
	var rec domain.BounceRecord
	_ = json.Unmarshal(js.published[0], &rec)
	if rec.BouncedRecipient != "bounced@example.com" {
		t.Errorf("BouncedRecipient: want bounced@example.com, got %q", rec.BouncedRecipient)
	}
	if !rec.BouncedAt.Equal(now) {
		t.Errorf("BouncedAt: want %v, got %v", now, rec.BouncedAt)
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
