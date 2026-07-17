package spam

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

type mockKV struct {
	data map[string][]byte
	fail bool
}

func newMockKV() *mockKV { return &mockKV{data: make(map[string][]byte)} }

func (m *mockKV) Create(key string, value []byte) (uint64, error) {
	if m.fail {
		return 0, errors.New("mock error")
	}
	if _, ok := m.data[key]; ok {
		return 0, nats.ErrKeyExists
	}
	m.data[key] = value
	return 1, nil
}

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

func TestCheck_KVCreateError(t *testing.T) {
	kv := newMockKV()
	kv.fail = true
	c := &Checker{kv: kv}
	err := c.Check("abc123")
	if err == nil {
		t.Fatal("expected error on KV create failure")
	}
	var valErr *domain.ValidationError
	if errors.As(err, &valErr) {
		t.Errorf("KV failure must not surface as ValidationError, got %v", valErr)
	}
}
