package msgraph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
)

const baseURL = "https://graph.microsoft.com/v1.0"

// Client is the MS Graph HTTP client with circuit breaker and retry.
type Client struct {
	http    *http.Client
	breaker *gobreaker.CircuitBreaker
	tokens  *tokenCache

	tenantID     string
	clientID     string
	clientSecret string
}

func NewClient(tenantID, clientID, clientSecret string) *Client {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "msgraph",
		MaxRequests: 1,
		Interval:    0,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			// Permanent errors don't count against the breaker
			_, isPerm := err.(*GraphPermanentError)
			return isPerm
		},
	})

	return &Client{
		http:         &http.Client{Timeout: 30 * time.Second},
		breaker:      cb,
		tokens:       &tokenCache{},
		tenantID:     tenantID,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (c *Client) do(ctx context.Context, req *http.Request) ([]byte, int, error) {
	token, err := c.tokens.get(ctx, c.tenantID, c.clientID, c.clientSecret)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	var body []byte
	var statusCode int

	_, cbErr := c.breaker.Execute(func() (any, error) {
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, &GraphTransientError{Cause: err}
		}
		defer func() { _ = resp.Body.Close() }()

		statusCode = resp.StatusCode
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, &GraphTransientError{Cause: fmt.Errorf("read response: %w", err)}
		}

		switch {
		case statusCode == http.StatusTooManyRequests:
			return nil, &GraphTransientError{StatusCode: statusCode}
		case statusCode >= 500:
			return nil, &GraphTransientError{StatusCode: statusCode}
		case statusCode >= 400:
			return nil, &GraphPermanentError{StatusCode: statusCode, Body: string(body)}
		}
		return nil, nil
	})

	if cbErr != nil {
		return nil, statusCode, cbErr
	}
	return body, statusCode, nil
}

// doWithRetry retries on 5xx with Fibonacci backoff (max 2 retries).
func (c *Client) doWithRetry(ctx context.Context, buildReq func() (*http.Request, error)) ([]byte, int, error) {
	delays := []time.Duration{2 * time.Second, 2 * time.Second}
	for attempt := range 3 {
		req, err := buildReq()
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}
		body, status, err := c.do(ctx, req)
		if err == nil {
			return body, status, nil
		}
		var transient *GraphTransientError
		if _, ok := err.(*GraphTransientError); !ok {
			return nil, status, err
		}
		_ = transient
		if attempt < len(delays) {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(delays[attempt]):
			}
		}
	}
	return nil, 0, &GraphTransientError{Cause: fmt.Errorf("max retries exceeded")}
}
