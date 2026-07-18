package quota

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/testkit"
)

func TestCheck_UnderLimit(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	if err := checker.Check("tenant1", 100, 5); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheck_Unlimited(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	if err := checker.Check("tenant1", 0, 9999); err != nil {
		t.Fatalf("unlimited quota must always pass: %v", err)
	}
	if err := checker.Check("tenant1", -1, 9999); err != nil {
		t.Fatalf("unlimited quota (-1) must always pass: %v", err)
	}
}

func TestCheck_ExceedsLimit(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	_ = checker.Check("tenant1", 10, 8)
	err := checker.Check("tenant1", 10, 5)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("expected QuotaError, got %T: %v", err, err)
	}
	if quotaErr.Current != 8 || quotaErr.Requested != 5 || quotaErr.Limit != 10 {
		t.Errorf("unexpected QuotaError values: %+v", quotaErr)
	}
}

func TestCheck_Accumulates(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	for i := range 5 {
		if err := checker.Check("tenant1", 100, 10); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	err := checker.Check("tenant1", 100, 60)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("expected QuotaError after accumulation, got %T: %v", err, err)
	}
}

func TestCheck_ExpiredEntriesIgnored(t *testing.T) {
	kv := testkit.NewMockKV()
	// pre-populate with a 25h-old entry
	old := state{Entries: []entry{{TS: time.Now().Add(-25 * time.Hour).Unix(), Count: 99}}}
	data, _ := json.Marshal(old)
	kv.Data["tenant1"] = data
	kv.Revisions["tenant1"] = 1

	checker := &Checker{kv: kv}
	if err := checker.Check("tenant1", 10, 5); err != nil {
		t.Fatalf("old entries should be ignored, got: %v", err)
	}
}

func TestCheck_KVErrorFailClosed(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.GetErr = errors.New("mock KV error")
	checker := &Checker{kv: kv}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("expected QuotaStateError on KV failure, got %T: %v", err, err)
	}
}

// alwaysCASConflictKV always returns a JetStream CAS conflict on write,
// forcing Check() to exhaust all maxCASRetries.
type alwaysCASConflictKV struct{}

func (m *alwaysCASConflictKV) Get(_ string) (nats.KeyValueEntry, error) {
	return nil, nats.ErrKeyNotFound
}

func (m *alwaysCASConflictKV) Create(_ string, _ []byte) (uint64, error) {
	return 0, &testkit.WrongSeqError{}
}

func (m *alwaysCASConflictKV) Update(_ string, _ []byte, _ uint64) (uint64, error) {
	return 0, &testkit.WrongSeqError{}
}

func TestCheck_CASRetryExhausted(t *testing.T) {
	checker := &Checker{kv: &alwaysCASConflictKV{}}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("expected QuotaStateError after CAS exhaustion, got %T: %v", err, err)
	}
}
