package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/natsutil"
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

func provisionWorker(t *testing.T, js nats.JetStreamContext) nats.KeyValue {
	t.Helper()
	if err := natsutil.Setup(js, time.Hour); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := natsutil.ProvisionWorkerConsumer(js, time.Second, 8); err != nil {
		t.Fatalf("provision consumer: %v", err)
	}
	kv, err := js.KeyValue(natsutil.BucketDelivered)
	if err != nil {
		t.Fatalf("delivered KV: %v", err)
	}
	return kv
}

func TestConsumerRun_SubscribeError(t *testing.T) {
	_, js := testNATS(t)
	c := NewConsumer(js, &Processor{})
	if err := c.Run(context.Background()); err == nil {
		t.Fatal("Run must fail when DISPATCH_MAILS consumer is missing")
	}
}

func TestConsumerRun_ContextCancel(t *testing.T) {
	_, js := testNATS(t)
	kv := provisionWorker(t, js)
	proc := NewProcessor(&stubGraph{}, kv, &captureJS{}, nil, 8, time.Second)
	c := NewConsumer(js, proc)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- c.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("cancel must return nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Consumer.Run did not return after cancel")
	}
}

func TestConsumerRun_TestModeAcks(t *testing.T) {
	_, js := testNATS(t)
	kv := provisionWorker(t, js)
	jsPub := &captureJS{}
	proc := NewProcessor(&stubGraph{err: errors.New("graph must not be called in test mode")}, kv, jsPub, nil, 8, time.Second)
	c := NewConsumer(js, proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- c.Run(ctx) }()

	traceID := "consumer-run-trace"
	payload, err := json.Marshal(domain.MailRequestDO{
		TraceID:    traceID,
		AppTag:     "app",
		Sender:     testSender,
		Recipients: []string{testRecipient},
		Subject:    "consumer run",
		Test:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.Publish(natsutil.SubjectMails, payload); err != nil {
		t.Fatalf("publish: %v", err)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := kv.Get(traceID); err == nil {
			cancel()
			select {
			case runErr := <-errCh:
				if runErr != nil {
					t.Fatalf("Run: %v", runErr)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not return after cancel")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	t.Fatal("Consumer.Run did not ack test-mode message into delivered KV")
}
