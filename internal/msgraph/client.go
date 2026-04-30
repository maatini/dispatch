package msgraph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			return nil, &GraphTransientError{StatusCode: statusCode, RetryAfter: retryAfter}
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

// doWithRetry retries on transient errors, honouring Retry-After on 429 (max 2 retries).
func (c *Client) doWithRetry(ctx context.Context, buildReq func() (*http.Request, error)) ([]byte, int, error) {
	const fallbackDelay = 2 * time.Second
	const maxDelay = 30 * time.Second
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
		if !errors.As(err, &transient) {
			return nil, status, err
		}
		if attempt < 2 {
			wait := fallbackDelay
			if transient.RetryAfter > wait {
				wait = transient.RetryAfter
			}
			if wait > maxDelay {
				wait = maxDelay
			}
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return nil, 0, &GraphTransientError{Cause: fmt.Errorf("max retries exceeded")}
}

// parseRetryAfter parses the Retry-After header value (integer seconds).
func parseRetryAfter(header string) time.Duration {
	secs, err := strconv.Atoi(header)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
