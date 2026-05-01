package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	authTestSecret = "test-secret-key"
	bearerPrefix   = "Bearer "
)

func signedToken(t *testing.T, secret string, expiry time.Time) string {
	t.Helper()
	claims := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(expiry)}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func TestAuthMiddleware(t *testing.T) {
	validToken := signedToken(t, authTestSecret, time.Now().Add(time.Hour))
	expiredToken := signedToken(t, authTestSecret, time.Now().Add(-time.Hour))
	wrongSecretToken := signedToken(t, "wrong-secret", time.Now().Add(time.Hour))

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid token", bearerPrefix + validToken, http.StatusOK},
		{"no token", "", http.StatusUnauthorized},
		{"wrong secret", bearerPrefix + wrongSecretToken, http.StatusUnauthorized},
		{"expired token", bearerPrefix + expiredToken, http.StatusUnauthorized},
		{"malformed token", "Bearer notajwt", http.StatusUnauthorized},
		{"missing bearer prefix", validToken, http.StatusUnauthorized},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := AuthMiddleware(authTestSecret)(next)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Errorf("want %d, got %d (body: %s)", tc.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAuthMiddleware_ResponseBody(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := AuthMiddleware(authTestSecret)(next)

	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}
	body := rr.Body.String()
	if body == "" {
		t.Error("expected non-empty response body on 401")
	}
}
