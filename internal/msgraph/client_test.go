package msgraph

import (
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
	}{
		{"10", 10 * time.Second},
		{"1", 1 * time.Second},
		{"0", 0},
		{"-1", 0},
		{"", 0},
		{"not-a-number", 0},
		{"1.5", 0},
	}
	for _, tc := range cases {
		got := parseRetryAfter(tc.header)
		if got != tc.want {
			t.Errorf("parseRetryAfter(%q): want %v, got %v", tc.header, tc.want, got)
		}
	}
}

func TestGetToken_MockToken(t *testing.T) {
	c := &Client{mockToken: "test-token"}
	tok, err := c.getToken(nil) //nolint:staticcheck // nil ctx intentional for mock path
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "test-token" {
		t.Errorf("want test-token, got %s", tok)
	}
}

func TestBuildTransport_EmptyProxy(t *testing.T) {
	tr := buildTransport("")
	if tr == nil {
		t.Fatal("expected non-nil transport for empty proxy")
	}
}

func TestBuildTransport_WithProxy(t *testing.T) {
	tr := buildTransport("http://localhost:8000")
	if tr == nil {
		t.Fatal("expected non-nil transport for proxy URL")
	}
}
