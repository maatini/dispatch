package msgraph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sony/gobreaker"
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

func TestDo_BreakerOpenIsTransient(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// First 5 calls: 500 transient — these trip the breaker
		if callCount <= 5 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// 6th call should never happen because breaker is open
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "test-breaker",
		MaxRequests: 1,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})

	c := &Client{
		http:      &http.Client{},
		breaker:   cb,
		mockToken: "test-token",
	}

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		_, _, err := c.do(context.Background(), req)
		if err != nil {
			var transient *GraphTransientError
			if !errors.As(err, &transient) {
				t.Fatalf("attempt %d: expected transient error, got %T: %v", i+1, err, err)
			}
		}
	}

	// 6th call: breaker should be open → ErrOpenState → wrapped as transient
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, _, err := c.do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when breaker is open")
	}
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("breaker-open error must be *GraphTransientError, got %T: %v", err, err)
	}
}
