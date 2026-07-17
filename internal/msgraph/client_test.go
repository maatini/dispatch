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

func TestDo_GetTokenErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		http:   &http.Client{},
		tokens: &tokenCache{}, // no credentials → will fail token fetch
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, _, err := c.do(context.Background(), req)
	if err == nil {
		t.Fatal("expected token fetch error to propagate")
	}
}

func TestDo_429_RetryAfterInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test-429"})
	c := &Client{
		http:      &http.Client{},
		breaker:   cb,
		mockToken: "test-token",
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, _, err := c.do(context.Background(), req)
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("expected GraphTransientError, got %T: %v", err, err)
	}
	if transient.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter: want 30s, got %v", transient.RetryAfter)
	}
	if transient.StatusCode != 429 {
		t.Errorf("StatusCode: want 429, got %d", transient.StatusCode)
	}
}

func TestDo_5xx_ReturnsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test-5xx"})
	c := &Client{
		http:      &http.Client{},
		breaker:   cb,
		mockToken: "test-token",
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, _, err := c.do(context.Background(), req)
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("expected GraphTransientError for 502, got %T: %v", err, err)
	}
}

func TestDo_4xx_ReturnsPermanentWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid"))
	}))
	defer srv.Close()

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "test-4xx"})
	c := &Client{
		http:      &http.Client{},
		breaker:   cb,
		mockToken: "test-token",
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, _, err := c.do(context.Background(), req)
	var perm *GraphPermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("expected GraphPermanentError for 400, got %T: %v", err, err)
	}
	if perm.StatusCode != 400 {
		t.Errorf("StatusCode: want 400, got %d", perm.StatusCode)
	}
	if perm.Body != "invalid" {
		t.Errorf("Body: want 'invalid', got %q", perm.Body)
	}
}

func TestDoWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		http:           &http.Client{},
		breaker:        gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "retry"}),
		mockToken:      "test-token",
		retryBaseDelay: 1 * time.Millisecond,
	}
	_, status, err := c.doWithRetry(context.Background(), func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	})
	if err != nil {
		t.Fatalf("expected success on second attempt, got: %v", err)
	}
	if status != 200 {
		t.Errorf("status: want 200, got %d", status)
	}
	if calls != 2 {
		t.Errorf("calls: want 2, got %d", calls)
	}
}

func TestDoWithRetry_MaxRetriesExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := &Client{
		http:           &http.Client{},
		breaker:        gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "max"}),
		mockToken:      "test-token",
		retryBaseDelay: 1 * time.Millisecond,
	}
	_, _, err := c.doWithRetry(context.Background(), func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	})
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Fatalf("expected GraphTransientError after max retries, got %T: %v", err, err)
	}
}

func TestDoWithRetry_NoRetryOnPermanentError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := &Client{
		http:           &http.Client{},
		breaker:        gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "perm"}),
		mockToken:      "test-token",
		retryBaseDelay: 1 * time.Millisecond,
	}
	_, _, err := c.doWithRetry(context.Background(), func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	})
	var perm *GraphPermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("expected GraphPermanentError, got %T: %v", err, err)
	}
	if calls != 1 {
		t.Errorf("permanent error must not retry; calls: want 1, got %d", calls)
	}
}

func TestDoWithRetry_ContextDoneDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := &Client{
		http:           &http.Client{},
		breaker:        gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "ctx"}),
		mockToken:      "test-token",
		retryBaseDelay: 10 * time.Second, // long delay forces ctx cancel
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before call

	_, _, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	})
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context error, got: %v", err)
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
