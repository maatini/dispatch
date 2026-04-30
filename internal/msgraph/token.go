package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type tokenCache struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (tc *tokenCache) get(ctx context.Context, tenantID, clientID, clientSecret string) (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.accessToken != "" && time.Now().Before(tc.expiresAt) {
		return tc.accessToken, nil
	}

	token, expiresIn, err := fetchToken(ctx, tenantID, clientID, clientSecret)
	if err != nil {
		return "", err
	}

	tc.accessToken = token
	tc.expiresAt = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return tc.accessToken, nil
}

func fetchToken(ctx context.Context, tenantID, clientID, clientSecret string) (string, int, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("scope", "https://graph.microsoft.com/.default")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, &GraphTransientError{Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, &GraphTransientError{StatusCode: resp.StatusCode, Cause: fmt.Errorf("token fetch: %s", body)}
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, fmt.Errorf("parse token response: %w", err)
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}
