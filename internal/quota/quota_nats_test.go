package quota

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

func testNATS(t *testing.T) nats.JetStreamContext {
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
	return js
}

func testQuotaKV(t *testing.T) nats.KeyValue {
	t.Helper()
	js := testNATS(t)
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: "quota", History: 1})
	if err != nil {
		t.Fatalf("create kv: %v", err)
	}
	return kv
}

func TestCheck_EmbeddedNATS_ExactLimit(t *testing.T) {
	checker := NewChecker(testQuotaKV(t))
	if err := checker.Check("tenant1", 10, 10); err != nil {
		t.Fatalf("requested == limit against real KV must be allowed: %v", err)
	}
	err := checker.Check("tenant1", 10, 1)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("over exact limit must be QuotaError, got %T: %v", err, err)
	}
}

func TestCheck_EmbeddedNATS_ConcurrentDoesNotExceed(t *testing.T) {
	checker := &Checker{kv: testQuotaKV(t), retryPause: func(int) { time.Sleep(time.Millisecond) }}
	const limit = 10
	const workers = 20

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- checker.Check("tenant1", limit, 1)
		}()
	}
	wg.Wait()
	close(errCh)

	var admitted, exceeded, state int
	for err := range errCh {
		switch {
		case err == nil:
			admitted++
		case errors.As(err, new(*domain.QuotaError)):
			exceeded++
		case errors.As(err, new(*domain.QuotaStateError)):
			state++
		default:
			t.Errorf("unexpected error: %T %v", err, err)
		}
	}
	if admitted > limit {
		t.Fatalf("fail-closed broken: %d admitted on limit %d", admitted, limit)
	}
	if admitted != limit {
		t.Errorf("want exactly %d admitted, got admitted=%d exceeded=%d state=%d",
			limit, admitted, exceeded, state)
	}
}
