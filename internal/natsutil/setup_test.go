package natsutil

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func testNATS(t *testing.T) (*server.Server, nats.JetStreamContext) {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv.Start()
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL(), nats.Timeout(2*time.Second))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	return srv, js
}

func TestConnect_InvalidURL(t *testing.T) {
	_, _, err := Connect("nats://127.0.0.1:1")
	if err == nil {
		t.Skip("port 1 is reachable on this system, skipping")
	}
}

func TestProvisionStreams_CreatesFourStreams(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("ProvisionStreams: %v", err)
	}

	names := []string{StreamMails, StreamAudit, StreamDeadLetter, StreamBounces}
	for _, name := range names {
		info, err := js.StreamInfo(name)
		if err != nil {
			t.Fatalf("stream %s not found after provision: %v", name, err)
		}
		if info.Config.Name != name {
			t.Errorf("stream name mismatch for %s", name)
		}
	}
}

func TestProvisionStreams_Idempotent(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("first ProvisionStreams: %v", err)
	}
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("second ProvisionStreams must be idempotent: %v", err)
	}
}

func TestProvisionStreams_UpdatePath(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("ProvisionStreams: %v", err)
	}

	// Change the stream config manually and verify re-provision updates it back
	_, err := js.UpdateStream(&nats.StreamConfig{
		Name:     StreamAudit,
		Subjects: []string{SubjectAudit},
		Storage:  nats.FileStorage,
		MaxAge:   31 * 24 * time.Hour, // was 30 days
	})
	if err != nil {
		t.Fatalf("update stream manually: %v", err)
	}

	// Re-provision: update path applies original config
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("ProvisionStreams after manual update: %v", err)
	}

	info, err := js.StreamInfo(StreamAudit)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if info.Config.MaxAge != 30*24*time.Hour {
		t.Errorf("upsert must apply original config (30d), got %v", info.Config.MaxAge)
	}
}

func TestProvisionKVBuckets_BucketsExist(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionKVBuckets(js, time.Hour); err != nil {
		t.Fatalf("ProvisionKVBuckets: %v", err)
	}

	names := []string{BucketSenders, BucketQuota, BucketSpam, BucketDelivered}
	for _, name := range names {
		_, err := js.KeyValue(name)
		if err != nil {
			t.Errorf("KV bucket %s not found: %v", name, err)
		}
	}
}

func TestProvisionKVBuckets_SpamTTL(t *testing.T) {
	_, js := testNATS(t)
	wantTTL := 23*time.Hour + 17*time.Minute
	if err := ProvisionKVBuckets(js, wantTTL); err != nil {
		t.Fatalf("ProvisionKVBuckets: %v", err)
	}

	kv, err := js.KeyValue(BucketSpam)
	if err != nil {
		t.Fatalf("spam KV: %v", err)
	}
	status, err := kv.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	// TTL is stored in nanoseconds; allow small rounding tolerance
	got := status.TTL()
	if got.Round(time.Second) != wantTTL.Round(time.Second) {
		t.Errorf("spam TTL: want %v, got %v", wantTTL, got)
	}
}

func TestProvisionKVBuckets_Idempotent(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionKVBuckets(js, time.Hour); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := ProvisionKVBuckets(js, 2*time.Hour); err != nil {
		t.Fatalf("second must be idempotent: %v", err)
	}
}

func TestProvisionObjectStore_Creates(t *testing.T) {
	_, js := testNATS(t)
	store, err := ProvisionObjectStore(js)
	if err != nil {
		t.Fatalf("ProvisionObjectStore: %v", err)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatalf("object store status: %v", err)
	}
	if status.Bucket() != BucketAttachments {
		t.Errorf("bucket name: want %s, got %s", BucketAttachments, status.Bucket())
	}
}

func TestProvisionObjectStore_Idempotent(t *testing.T) {
	_, js := testNATS(t)
	store1, err := ProvisionObjectStore(js)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	store2, err := ProvisionObjectStore(js)
	if err != nil {
		t.Fatalf("second must return existing: %v", err)
	}
	s1, _ := store1.Status()
	s2, _ := store2.Status()
	if s1 == nil || s2 == nil {
		t.Fatal("status must not be nil")
	}
}

func TestProvisionWorkerConsumer_Creates(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("ProvisionStreams: %v", err)
	}
	if err := ProvisionWorkerConsumer(js); err != nil {
		t.Fatalf("ProvisionWorkerConsumer: %v", err)
	}

	info, err := js.ConsumerInfo(StreamMails, ConsumerMailWorker)
	if err != nil {
		t.Fatalf("consumer info: %v", err)
	}
	if info.Config.Durable != ConsumerMailWorker {
		t.Errorf("durable name: want %s, got %s", ConsumerMailWorker, info.Config.Durable)
	}
}

func TestProvisionWorkerConsumer_Idempotent(t *testing.T) {
	_, js := testNATS(t)
	if err := ProvisionStreams(js); err != nil {
		t.Fatalf("ProvisionStreams: %v", err)
	}
	if err := ProvisionWorkerConsumer(js); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := ProvisionWorkerConsumer(js); err != nil {
		t.Fatalf("second must be idempotent: %v", err)
	}
}
