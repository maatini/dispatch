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
