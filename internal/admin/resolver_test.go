package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
)

type capturePublisher struct {
	msgs []*nats.Msg
	err  error
}

func (c *capturePublisher) PublishMsg(msg *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	if c.err != nil {
		return nil, c.err
	}
	c.msgs = append(c.msgs, msg)
	return &nats.PubAck{}, nil
}

func TestReprocessDeadLetter_PreservesTraceIDHeader(t *testing.T) {
	pub := &capturePublisher{}
	r := &Resolver{mailPublisher: pub}

	payloads := []string{
		`{"traceId":"trace-aaa","appTag":"app1","sender":"a@example.com","subject":"s1","recipients":["r1@example.com"],"bodyContent":"b"}`,
		`{"traceId":"trace-bbb","appTag":"app2","sender":"a@example.com","subject":"s2","recipients":["r2@example.com"],"bodyContent":"b"}`,
	}
	for _, p := range payloads {
		if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: p}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if len(pub.msgs) != 2 {
		t.Fatalf("want 2 published messages, got %d", len(pub.msgs))
	}
	wantTrace := []string{"trace-aaa", "trace-bbb"}
	wantTag := []string{"app1", "app2"}
	for i, msg := range pub.msgs {
		if got := msg.Header.Get("traceId"); got != wantTrace[i] {
			t.Errorf("msg %d traceId header: want %s, got %s", i, wantTrace[i], got)
		}
		if got := msg.Header.Get("appTag"); got != wantTag[i] {
			t.Errorf("msg %d appTag header: want %s, got %s", i, wantTag[i], got)
		}
		if string(msg.Data) != payloads[i] {
			t.Errorf("msg %d data mismatch", i)
		}
	}
}

func TestReprocessDeadLetter_MissingTraceID_GeneratesUUID(t *testing.T) {
	pub := &capturePublisher{}
	r := &Resolver{mailPublisher: pub}

	payload := `{"appTag":"app1","sender":"a@example.com","subject":"s","recipients":["r@example.com"],"bodyContent":"b"}`
	if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: payload}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.msgs) != 1 {
		t.Fatalf("want 1 published message, got %d", len(pub.msgs))
	}
	got := pub.msgs[0].Header.Get("traceId")
	if got == "" || got == "unknown" {
		t.Errorf("expected generated traceId, got %q", got)
	}
}

func TestReprocessDeadLetter_InvalidPayload_Fails(t *testing.T) {
	pub := &capturePublisher{}
	r := &Resolver{mailPublisher: pub}

	if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: `not-json`}); err == nil {
		t.Fatal("expected error for invalid payload")
	}
	if len(pub.msgs) != 0 {
		t.Errorf("no message must be published for invalid payload, got %d", len(pub.msgs))
	}
}

func TestReprocessDeadLetter_PublishError(t *testing.T) {
	pub := &capturePublisher{err: errors.New("nats down")}
	r := &Resolver{mailPublisher: pub}

	payload := `{"traceId":"t1","appTag":"app1"}`
	if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: payload}); err == nil {
		t.Fatal("expected publish error to propagate")
	}
}
