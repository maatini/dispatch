package spam

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

type mockKV struct {
	data map[string][]byte
	fail bool
}

func newMockKV() *mockKV { return &mockKV{data: make(map[string][]byte)} }

func (m *mockKV) Get(key string) (nats.KeyValueEntry, error) {
	if m.fail {
		return nil, errors.New("mock error")
	}
	v, ok := m.data[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return &mockEntry{value: v}, nil
}

func (m *mockKV) Put(key string, value []byte) (uint64, error) {
	if m.fail {
		return 0, errors.New("mock error")
	}
	m.data[key] = value
	return 1, nil
}

type mockEntry struct{ value []byte }

func (e *mockEntry) Bucket() string             { return "" }
func (e *mockEntry) Key() string                { return "" }
func (e *mockEntry) Value() []byte              { return e.value }
func (e *mockEntry) Revision() uint64           { return 0 }
func (e *mockEntry) Delta() uint64              { return 0 }
func (e *mockEntry) Created() time.Time         { return time.Time{} }
func (e *mockEntry) Operation() nats.KeyValueOp { return nats.KeyValuePut }

func TestCheck_FirstSeen(t *testing.T) {
	c := &Checker{kv: newMockKV()}
	if err := c.Check("abc123"); err != nil {
		t.Fatalf("first occurrence must pass: %v", err)
	}
}

func TestCheck_DuplicateDetected(t *testing.T) {
	c := &Checker{kv: newMockKV()}
	_ = c.Check("abc123")
	err := c.Check("abc123")
	var valErr *domain.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if valErr.Code != domain.ErrSpamDetected {
		t.Errorf("expected ErrSpamDetected, got %s", valErr.Code)
	}
}

func TestCheck_DifferentHashesPass(t *testing.T) {
	c := &Checker{kv: newMockKV()}
	_ = c.Check("hash1")
	if err := c.Check("hash2"); err != nil {
		t.Fatalf("different hash must pass: %v", err)
	}
}

func TestCheck_KVGetError(t *testing.T) {
	kv := newMockKV()
	kv.fail = true
	c := &Checker{kv: kv}
	if err := c.Check("abc123"); err == nil {
		t.Fatal("expected error on KV get failure")
	}
}
