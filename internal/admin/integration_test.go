//go:build integration

package admin

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/natsutil"
)

func integrationNATS(t *testing.T) nats.JetStreamContext {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}
	nc, err := nats.Connect(url, nats.Timeout(2*time.Second))
	if err != nil {
		t.Skipf("NATS not reachable at %s: %v", url, err)
	}
	t.Cleanup(nc.Close)
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream context: %v", err)
	}
	if err := natsutil.ProvisionStreams(js); err != nil {
		t.Fatalf("provision streams: %v", err)
	}
	return js
}

func TestIntegration_MailsReturnsAllWellFormedRecords(t *testing.T) {
	js := integrationNATS(t)

	prefix := uuid.NewString()
	want := []string{prefix + "-1", prefix + "-2", prefix + "-3"}
	for _, traceID := range want {
		data, err := json.Marshal(domain.AuditRecord{
			TraceID:    traceID,
			AppTag:     "itest",
			Status:     domain.StatusDelivered,
			Sender:     "s@example.com",
			Recipients: []string{"r@example.com"},
			Timestamp:  time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := js.Publish(natsutil.SubjectAudit, data); err != nil {
			t.Fatalf("publish audit record: %v", err)
		}
	}
	if _, err := js.Publish(natsutil.SubjectAudit, []byte("corrupt-not-json")); err != nil {
		t.Fatalf("publish corrupt record: %v", err)
	}

	r := NewResolver(nil, js)
	resp, err := r.Mails(context.Background(), pagedMailArgs{})
	if err != nil {
		t.Fatalf("Mails: %v", err)
	}

	found := make(map[string]bool, len(want))
	for _, item := range resp.Items() {
		found[item.TraceId()] = true
	}
	for _, traceID := range want {
		if !found[traceID] {
			t.Errorf("audit record %s missing from Mails result", traceID)
		}
	}
}

func TestIntegration_MailsFilterByAppTag(t *testing.T) {
	js := integrationNATS(t)

	data, _ := json.Marshal(domain.AuditRecord{
		TraceID:    "filter-1",
		AppTag:     "filter-app",
		Status:     domain.StatusDelivered,
		Sender:     "s@example.com",
		Recipients: []string{"r@example.com"},
		Timestamp:  time.Now().UTC(),
	})
	if _, err := js.Publish(natsutil.SubjectAudit, data); err != nil {
		t.Fatalf("publish: %v", err)
	}
	data2, _ := json.Marshal(domain.AuditRecord{
		TraceID:    "filter-2",
		AppTag:     "other",
		Status:     domain.StatusDelivered,
		Sender:     "s@example.com",
		Recipients: []string{"r@example.com"},
		Timestamp:  time.Now().UTC(),
	})
	if _, err := js.Publish(natsutil.SubjectAudit, data2); err != nil {
		t.Fatalf("publish: %v", err)
	}

	r := NewResolver(nil, js)
	resp, err := r.Mails(context.Background(), pagedMailArgs{Filter: &mailFilterArgs{AppTag: strPtr("filter-app")}})
	if err != nil {
		t.Fatalf("Mails: %v", err)
	}
	if resp.Total() != 1 {
		t.Errorf("filter AppTag: want 1, got %d", resp.Total())
	}
}

func TestIntegration_MailsPagination(t *testing.T) {
	js := integrationNATS(t)

	prefix := uuid.NewString()
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(domain.AuditRecord{
			TraceID:    prefix + "-" + string(rune('a'+i)),
			AppTag:     "page-app",
			Status:     domain.StatusDelivered,
			Sender:     "s@example.com",
			Recipients: []string{"r@example.com"},
			Timestamp:  time.Now().UTC(),
		})
		if _, err := js.Publish(natsutil.SubjectAudit, data); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	r := NewResolver(nil, js)
	resp, err := r.Mails(context.Background(), pagedMailArgs{Filter: &mailFilterArgs{AppTag: strPtr("page-app")}, Page: ptr(0), Size: ptr(2)})
	if err != nil {
		t.Fatalf("Mails: %v", err)
	}
	if len(resp.Items()) != 2 {
		t.Errorf("page 0 size 2: want 2 items, got %d", len(resp.Items()))
	}
	if resp.Total() != 5 {
		t.Errorf("total: want 5, got %d", resp.Total())
	}
}

func TestIntegration_BouncesReturnsRecords(t *testing.T) {
	js := integrationNATS(t)

	data, _ := json.Marshal(domain.BounceRecord{
		OriginalTraceID:  "bounce-1",
		BouncedAt:        time.Now().UTC(),
		BounceReason:     "blocked",
		BouncedRecipient: "r@example.com",
		ProcessedAt:      time.Now().UTC(),
	})
	if _, err := js.Publish(natsutil.SubjectBounce, data); err != nil {
		t.Fatalf("publish bounce: %v", err)
	}

	r := NewResolver(nil, js)
	resp, err := r.Bounces(context.Background(), pagedBounceArgs{})
	if err != nil {
		t.Fatalf("Bounces: %v", err)
	}
	if resp.Total() < 1 {
		t.Errorf("expected at least 1 bounce, got %d", resp.Total())
	}
}

func TestIntegration_DeadLettersReturnsRecords(t *testing.T) {
	js := integrationNATS(t)

	data, _ := json.Marshal(domain.DeadLetter{
		Payload:   `{"invalid"`,
		Error:     "parse error",
		Timestamp: time.Now().UTC(),
	})
	if _, err := js.Publish(natsutil.SubjectDeadLetter, data); err != nil {
		t.Fatalf("publish dead letter: %v", err)
	}

	r := NewResolver(nil, js)
	resp, err := r.DeadLetters(context.Background(), pagedBounceArgs{})
	if err != nil {
		t.Fatalf("DeadLetters: %v", err)
	}
	if resp.Total() < 1 {
		t.Errorf("expected at least 1 dead letter, got %d", resp.Total())
	}
}

func TestIntegration_ReadStreamNonExistent(t *testing.T) {
	r := NewResolver(nil, nil)
	_, err := r.Mails(context.Background(), pagedMailArgs{})
	if err == nil {
		t.Error("expected error for nil JetStream context")
	}
}
