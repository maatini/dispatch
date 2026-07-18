package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"dispatch/internal/loggy"
)

var authLog = loggy.GetLogger("AuthMiddleware")

// AuthMiddleware validates Bearer JWTs signed with HMAC-SHA256.
// Requests without a valid token receive 401; /health must be registered outside this middleware.
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr, ok := bearerToken(r)
			if !ok {
				writeUnauthorized(w, r)
				return
			}

			_, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(secret), nil
			}, jwt.WithExpirationRequired())
			if err != nil {
				writeUnauthorized(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token, found && token != ""
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	authLog.Warn("unauthorized request",
		loggy.Kv("method", r.Method),
		loggy.Kv("path", r.URL.Path),
	)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  401,
		"code":    "UNAUTHORIZED",
		"message": "invalid or missing token",
	})
}
