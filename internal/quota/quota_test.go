package quota

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

// mockKV is a minimal in-memory KV implementation for testing.
type mockKV struct {
	data     map[string][]byte
	revision map[string]uint64
	failNext bool
}

func newMockKV() *mockKV {
	return &mockKV{
		data:     make(map[string][]byte),
		revision: make(map[string]uint64),
	}
}

func (m *mockKV) Get(key string) (nats.KeyValueEntry, error) {
	if m.failNext {
		return nil, errors.New("mock KV error")
	}
	v, ok := m.data[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return &mockEntry{value: v, revision: m.revision[key]}, nil
}

func (m *mockKV) Create(key string, value []byte) (uint64, error) {
	if _, ok := m.data[key]; ok {
		return 0, nats.ErrKeyExists
	}
	m.data[key] = value
	m.revision[key] = 1
	return 1, nil
}

// mockWrongSeqErr implements nats.JetStreamError to simulate CAS conflict.
type mockWrongSeqErr struct{}

func (e *mockWrongSeqErr) APIError() *nats.APIError { return &nats.APIError{Code: 400} }
func (e *mockWrongSeqErr) Error() string            { return "wrong last sequence" }

func (m *mockKV) Update(key string, value []byte, last uint64) (uint64, error) {
	if m.revision[key] != last {
		return 0, &mockWrongSeqErr{}
	}
	m.data[key] = value
	m.revision[key] = last + 1
	return last + 1, nil
}

type mockEntry struct {
	value    []byte
	revision uint64
}

func (e *mockEntry) Bucket() string             { return "" }
func (e *mockEntry) Key() string                { return "" }
func (e *mockEntry) Value() []byte              { return e.value }
func (e *mockEntry) Revision() uint64           { return e.revision }
func (e *mockEntry) Delta() uint64              { return 0 }
func (e *mockEntry) Created() time.Time         { return time.Time{} }
func (e *mockEntry) Operation() nats.KeyValueOp { return nats.KeyValuePut }

func TestCheck_UnderLimit(t *testing.T) {
	checker := &Checker{kv: newMockKV()}
	if err := checker.Check("tenant1", 100, 5); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheck_Unlimited(t *testing.T) {
	checker := &Checker{kv: newMockKV()}
	if err := checker.Check("tenant1", 0, 9999); err != nil {
		t.Fatalf("unlimited quota must always pass: %v", err)
	}
	if err := checker.Check("tenant1", -1, 9999); err != nil {
		t.Fatalf("unlimited quota (-1) must always pass: %v", err)
	}
}

func TestCheck_ExceedsLimit(t *testing.T) {
	checker := &Checker{kv: newMockKV()}
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
	checker := &Checker{kv: newMockKV()}
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
	kv := newMockKV()
	// pre-populate with a 25h-old entry
	old := state{Entries: []entry{{TS: time.Now().Add(-25 * time.Hour).Unix(), Count: 99}}}
	data, _ := json.Marshal(old)
	kv.data["tenant1"] = data
	kv.revision["tenant1"] = 1

	checker := &Checker{kv: kv}
	if err := checker.Check("tenant1", 10, 5); err != nil {
		t.Fatalf("old entries should be ignored, got: %v", err)
	}
}

func TestCheck_KVErrorFailClosed(t *testing.T) {
	kv := newMockKV()
	kv.failNext = true
	checker := &Checker{kv: kv}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("expected QuotaStateError on KV failure, got %T: %v", err, err)
	}
}

func TestCurrentUsage_Empty(t *testing.T) {
	checker := &Checker{kv: newMockKV()}
	n, err := checker.CurrentUsage("tenant1")
	if err != nil || n != 0 {
		t.Fatalf("expected 0, nil; got %d, %v", n, err)
	}
}

func TestCurrentUsage_Sum(t *testing.T) {
	checker := &Checker{kv: newMockKV()}
	_ = checker.Check("tenant1", 1000, 5)
	_ = checker.Check("tenant1", 1000, 3)
	n, err := checker.CurrentUsage("tenant1")
	if err != nil || n != 8 {
		t.Fatalf("expected 8, nil; got %d, %v", n, err)
	}
}

func TestCurrentUsage_ExpiredNotCounted(t *testing.T) {
	kv := newMockKV()
	old := state{Entries: []entry{{TS: time.Now().Add(-25 * time.Hour).Unix(), Count: 99}}}
	data, _ := json.Marshal(old)
	kv.data["tenant1"] = data
	kv.revision["tenant1"] = 1
	checker := &Checker{kv: kv}
	n, err := checker.CurrentUsage("tenant1")
	if err != nil || n != 0 {
		t.Fatalf("expired entries must not count; got %d, %v", n, err)
	}
}

func TestCurrentUsage_KVError(t *testing.T) {
	kv := newMockKV()
	kv.failNext = true
	checker := &Checker{kv: kv}
	_, err := checker.CurrentUsage("tenant1")
	if err == nil {
		t.Fatal("expected error on KV failure")
	}
}

// alwaysCASConflictKV always returns a JetStream CAS conflict on write,
// forcing Check() to exhaust all maxCASRetries.
type alwaysCASConflictKV struct{}

func (m *alwaysCASConflictKV) Get(_ string) (nats.KeyValueEntry, error) {
	return nil, nats.ErrKeyNotFound
}

func (m *alwaysCASConflictKV) Create(_ string, _ []byte) (uint64, error) {
	return 0, &mockWrongSeqErr{}
}

func (m *alwaysCASConflictKV) Update(_ string, _ []byte, _ uint64) (uint64, error) {
	return 0, &mockWrongSeqErr{}
}

func TestCheck_CASRetryExhausted(t *testing.T) {
	checker := &Checker{kv: &alwaysCASConflictKV{}}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("expected QuotaStateError after CAS exhaustion, got %T: %v", err, err)
	}
}
