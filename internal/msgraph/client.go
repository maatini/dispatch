package msgraph

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sony/gobreaker"

	"dispatch/internal/loggy"
)

var clientLog = loggy.GetLogger("MSGraphClient")

const baseURL = "https://graph.microsoft.com/v1.0"

// Client is the MS Graph HTTP client with circuit breaker and retry.
type Client struct {
	http    *http.Client
	breaker *gobreaker.CircuitBreaker
	tokens  *tokenCache

	tenantID       string
	clientID       string
	clientSecret   string
	mockToken      string        // non-empty → skip OAuth2, use this token directly
	retryBaseDelay time.Duration // fallback delay between retries; default 2s
}

func NewClient(tenantID, clientID, clientSecret, proxyURL, mockToken string) *Client {
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
		http:           &http.Client{Timeout: 30 * time.Second, Transport: buildTransport(proxyURL)},
		breaker:        cb,
		tokens:         &tokenCache{},
		tenantID:       tenantID,
		clientID:       clientID,
		clientSecret:   clientSecret,
		mockToken:      mockToken,
		retryBaseDelay: 2 * time.Second,
	}
}

// buildTransport returns a transport that routes through proxyURL with TLS verification
// disabled — intended only for local dev proxy use.
func buildTransport(proxyURL string) http.RoundTripper {
	if proxyURL == "" {
		return http.DefaultTransport
	}
	u, _ := url.Parse(proxyURL)
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyURL(u)
	t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // dev proxy only
	return t
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	if c.mockToken != "" {
		return c.mockToken, nil
	}
	return c.tokens.get(ctx, c.tenantID, c.clientID, c.clientSecret)
}

func (c *Client) do(ctx context.Context, req *http.Request) ([]byte, int, error) {
	token, err := c.getToken(ctx)
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
		if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
			return nil, statusCode, &GraphTransientError{Cause: cbErr}
		}
		return nil, statusCode, cbErr
	}
	return body, statusCode, nil
}

// doWithRetry retries on transient errors, honouring Retry-After on 429 (max 2 retries).
func (c *Client) doWithRetry(ctx context.Context, buildReq func() (*http.Request, error)) ([]byte, int, error) {
	const maxDelay = 30 * time.Second
	clientLog.RecordApiStart("MS_GRAPH")
	for attempt := range 3 {
		req, err := buildReq()
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}
		body, status, err := c.do(ctx, req)
		if err == nil {
			clientLog.ExternalApiSuccess("MS_GRAPH", status)
			return body, status, nil
		}
		var transient *GraphTransientError
		if !errors.As(err, &transient) {
			clientLog.ApiClientError("MS_GRAPH", status, err.Error())
			return nil, status, err
		}
		if attempt < 2 {
			clientLog.Warn("MS Graph transient error, retrying",
				loggy.Kv("attempt", attempt+1),
				loggy.Kv("status", status),
			)
			wait := c.retryBaseDelay
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
	finalErr := &GraphTransientError{Cause: fmt.Errorf("max retries exceeded")}
	clientLog.ExternalApiFailure("MS_GRAPH", 0, finalErr)
	return nil, 0, finalErr
}

// parseRetryAfter parses the Retry-After header value (integer seconds).
func parseRetryAfter(header string) time.Duration {
	secs, err := strconv.Atoi(header)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
