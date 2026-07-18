package spam

import (
	"errors"
	"testing"

	"dispatch/internal/domain"
	"dispatch/internal/testkit"
)

const (
	hashTestAppTag = "sunshine-app"
	hashTestEmail  = "a@b.com"
)

func TestHash(t *testing.T) {
	h1 := Hash(hashTestAppTag, "Hello", []string{hashTestEmail}, 100, 0)
	h2 := Hash(hashTestAppTag, "Hello", []string{hashTestEmail}, 100, 0)
	if h1 != h2 {
		t.Error("identical inputs must produce identical hash")
	}

	h3 := Hash(hashTestAppTag, "Hello", []string{hashTestEmail}, 101, 0)
	if h1 == h3 {
		t.Error("different body length must produce different hash")
	}

	h4 := Hash("other-tag", "Hello", []string{hashTestEmail}, 100, 0)
	if h1 == h4 {
		t.Error("different appTag must produce different hash")
	}

	if len(h1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(h1))
	}
}

func TestCheck_FirstSeen(t *testing.T) {
	c := &Checker{kv: testkit.NewMockKV()}
	if err := c.Check("abc123"); err != nil {
		t.Fatalf("first occurrence must pass: %v", err)
	}
}

func TestCheck_DuplicateDetected(t *testing.T) {
	c := &Checker{kv: testkit.NewMockKV()}
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
	c := &Checker{kv: testkit.NewMockKV()}
	_ = c.Check("hash1")
	if err := c.Check("hash2"); err != nil {
		t.Fatalf("different hash must pass: %v", err)
	}
}

func TestCheck_KVCreateError(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.CreateErr = errors.New("mock error")
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
