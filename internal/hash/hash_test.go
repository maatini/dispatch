package hash

import "testing"

func TestSpamHash(t *testing.T) {
	h1 := SpamHash("sunshine-app", "Hello", []string{"a@b.com"}, 100, 0)
	h2 := SpamHash("sunshine-app", "Hello", []string{"a@b.com"}, 100, 0)
	if h1 != h2 {
		t.Error("identical inputs must produce identical hash")
	}

	h3 := SpamHash("sunshine-app", "Hello", []string{"a@b.com"}, 101, 0)
	if h1 == h3 {
		t.Error("different body length must produce different hash")
	}

	h4 := SpamHash("other-tag", "Hello", []string{"a@b.com"}, 100, 0)
	if h1 == h4 {
		t.Error("different appTag must produce different hash")
	}

	if len(h1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(h1))
	}
}
