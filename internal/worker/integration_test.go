//go:build integration

package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/msgraph"
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
	if err := natsutil.ProvisionKVBuckets(js, time.Hour); err != nil {
		t.Fatalf("provision KV buckets: %v", err)
	}
	return js
}

func TestIntegration_AttachmentRoundtrip(t *testing.T) {
	js := integrationNATS(t)
	objStore, err := natsutil.ProvisionObjectStore(js)
	if err != nil {
		t.Fatalf("provision object store: %v", err)
	}
	store := NewAttachmentStore(objStore)

	key := "itest/" + uuid.NewString()
	content := []byte("integration test pdf bytes")
	if _, err := objStore.Put(&nats.ObjectMeta{Name: key}, bytes.NewReader(content)); err != nil {
		t.Fatalf("object store put: %v", err)
	}

	fetched, err := store.Fetch([]domain.AttachmentDO{{Name: "f.pdf", ContentType: "application/pdf", ObjectKey: key}})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(fetched[0].Content, content) {
		t.Errorf("fetched content mismatch: want %q, got %q", content, fetched[0].Content)
	}

	store.Cleanup(fetched)
	if _, err := objStore.GetInfo(key); err == nil {
		t.Error("expected object to be deleted after cleanup")
	}
}

type transientGraphStub struct{}

func (g *transientGraphStub) SendEmail(_ context.Context, _ domain.MailRequestDO) error {
	return &msgraph.GraphTransientError{StatusCode: 500}
}

type recordingPublisher struct {
	js   nats.JetStreamContext
	mu   sync.Mutex
	data [][]byte
}

func (r *recordingPublisher) Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	r.mu.Lock()
	r.data = append(r.data, data)
	r.mu.Unlock()
	return r.js.Publish(subj, data, opts...)
}

func (r *recordingPublisher) contains(needle string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.data {
		if bytes.Contains(d, []byte(needle)) {
			return true
		}
	}
	return false
}

func TestIntegration_TransientError_Redelivers(t *testing.T) {
	js := integrationNATS(t)

	stream := "TEST_WORKER_REDELIVERY"
	subject := "test.worker.redelivery"
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:      stream,
		Subjects:  []string{subject},
		Storage:   nats.MemoryStorage,
		Retention: nats.WorkQueuePolicy,
	}); err != nil {
		t.Fatalf("add stream: %v", err)
	}
	t.Cleanup(func() { _ = js.DeleteStream(stream) })
	if _, err := js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       "test-worker",
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       2 * time.Second,
		FilterSubject: subject,
	}); err != nil {
		t.Fatalf("add consumer: %v", err)
	}

	deliveredKV, err := js.KeyValue(natsutil.BucketDelivered)
	if err != nil {
		t.Fatalf("delivered KV: %v", err)
	}
	rec := &recordingPublisher{js: js}
	proc := NewProcessor(&transientGraphStub{}, deliveredKV, rec, nil)

	traceID := uuid.NewString()
	payload, err := json.Marshal(domain.MailRequestDO{
		TraceID:    traceID,
		AppTag:     "itest",
		Sender:     "s@example.com",
		Recipients: []string{"r@example.com"},
		Subject:    "redelivery test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.Publish(subject, payload); err != nil {
		t.Fatalf("publish: %v", err)
	}

	sub, err := js.PullSubscribe(subject, "test-worker", nats.Bind(stream, "test-worker"))
	if err != nil {
		t.Fatalf("pull subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
	if err != nil || len(msgs) != 1 {
		t.Fatalf("first fetch: %v (got %d msgs)", err, len(msgs))
	}
	proc.Handle(context.Background(), msgs[0])

	msgs2, err := sub.Fetch(1, nats.MaxWait(10*time.Second))
	if err != nil || len(msgs2) != 1 {
		t.Fatalf("second fetch: %v (got %d msgs) — message was not redelivered", err, len(msgs2))
	}
	md, err := msgs2[0].Metadata()
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if md.NumDelivered < 2 {
		t.Errorf("expected NumDelivered >= 2, got %d", md.NumDelivered)
	}
	_ = msgs2[0].Ack()

	if rec.contains(traceID) {
		t.Error("transient error must not write audit or dead-letter records")
	}
}
