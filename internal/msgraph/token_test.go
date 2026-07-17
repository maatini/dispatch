package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func tokenServer(t *testing.T, expiresIn int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: fmt.Sprintf("tok%d", calls),
			ExpiresIn:   expiresIn,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestFetchToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "client_credentials" {
			http.Error(w, "wrong grant_type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok123", ExpiresIn: 3600})
	}))
	defer srv.Close()

	token, expiresIn, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok123" {
		t.Errorf("token: want tok123, got %s", token)
	}
	if expiresIn != 3600 {
		t.Errorf("expiresIn: want 3600, got %d", expiresIn)
	}
}

func TestFetchToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Errorf("want GraphTransientError, got %T", err)
	}
}

func TestFetchToken_ClientErrorIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var perm *GraphPermanentError
	if !errors.As(err, &perm) {
		t.Errorf("want GraphPermanentError for 400, got %T: %v", err, err)
	}
}

func TestFetchToken_UnauthorizedIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var perm *GraphPermanentError
	if !errors.As(err, &perm) {
		t.Errorf("want GraphPermanentError for 401, got %T: %v", err, err)
	}
}

func TestFetchToken_ServerErrorIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
	}))
	defer srv.Close()

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Errorf("want GraphTransientError for 504, got %T: %v", err, err)
	}
}

func TestFetchToken_TooManyRequestsIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Errorf("want GraphTransientError for 429, got %T: %v", err, err)
	}
}

func TestFetchToken_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
}

func TestTokenCache_CachesToken(t *testing.T) {
	srv, calls := tokenServer(t, 3600)
	tc := &tokenCache{tokenEndpointBase: srv.URL}

	for range 3 {
		tok, err := tc.get(context.Background(), "tenant", "cid", "csecret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tok != "tok1" {
			t.Errorf("token: want tok1, got %s", tok)
		}
	}
	if *calls != 1 {
		t.Errorf("expected 1 fetch, got %d", *calls)
	}
}

// TestTokenCache_ExpiryBuffer verifies the 60-second safety buffer (expiresIn-60, not expiresIn+60).
// With expiresIn=1, the buffer causes expiresAt = now-59s (already expired), so every call re-fetches.
// The INVERT_NEGATIVES mutant (expiresIn+60=61) would cache the token for 61s instead.
func TestTokenCache_ExpiryBuffer(t *testing.T) {
	srv, calls := tokenServer(t, 1)
	tc := &tokenCache{tokenEndpointBase: srv.URL}

	tok1, err := tc.get(context.Background(), "tenant", "cid", "csecret")
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	tok2, err := tc.get(context.Background(), "tenant", "cid", "csecret")
	if err != nil {
		t.Fatalf("second get: %v", err)
	}

	if *calls != 2 {
		t.Errorf("expected 2 fetches with expiresIn=1 (buffer makes token immediately stale), got %d", *calls)
	}
	if tok1 == tok2 {
		t.Error("expected different tokens on each call when expiry buffer exceeds expiresIn")
	}
}

func TestFetchToken_NetworkError_IsTransient(t *testing.T) {
	// Use a server that we close before the request to simulate network error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close immediately — request will fail

	_, _, err := fetchToken(context.Background(), srv.URL, "cid", "csecret")
	if err == nil {
		t.Fatal("expected network error")
	}
	var transient *GraphTransientError
	if !errors.As(err, &transient) {
		t.Errorf("want GraphTransientError for network error, got %T: %v", err, err)
	}
}

func TestTokenCache_RefreshErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tc := &tokenCache{tokenEndpointBase: srv.URL}
	_, err := tc.get(context.Background(), "tenant", "cid", "csecret")
	if err == nil {
		t.Fatal("expected refresh error to propagate")
	}
}
