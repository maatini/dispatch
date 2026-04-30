package config

import (
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("MS_GRAPH_TENANT_ID", "tenant")
	t.Setenv("MS_GRAPH_CLIENT_ID", "client")
	t.Setenv("MS_GRAPH_CLIENT_SECRET", "secret")
	t.Setenv("MS_GRAPH_SENDER_EMAIL", "sender@example.com")
}

func TestLoad_Success(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.NatsURL != "nats://localhost:4222" {
		t.Errorf("NatsURL: got %s", cfg.NatsURL)
	}
	if cfg.MSGraphSenderEmail != "sender@example.com" {
		t.Errorf("MSGraphSenderEmail: got %s", cfg.MSGraphSenderEmail)
	}
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default: want 8080, got %s", cfg.Port)
	}
	if cfg.SpamTimeoutSeconds != 60 {
		t.Errorf("SpamTimeoutSeconds default: want 60, got %d", cfg.SpamTimeoutSeconds)
	}
	if cfg.MaxBodySize != 10_000_000 {
		t.Errorf("MaxBodySize default: want 10000000, got %d", cfg.MaxBodySize)
	}
	if cfg.MaxTotalAttachmentMB != 20 {
		t.Errorf("MaxTotalAttachmentMB default: want 20, got %d", cfg.MaxTotalAttachmentMB)
	}
	if cfg.NatsPublishTimeout != 5*time.Second {
		t.Errorf("NatsPublishTimeout default: want 5s, got %v", cfg.NatsPublishTimeout)
	}
	if cfg.GraphRateLimiterSkip {
		t.Error("GraphRateLimiterSkip default: want false")
	}
}

func TestLoad_MissingNatsURL(t *testing.T) {
	// leave NATS_URL unset
	t.Setenv("MS_GRAPH_TENANT_ID", "tenant")
	t.Setenv("MS_GRAPH_CLIENT_ID", "client")
	t.Setenv("MS_GRAPH_CLIENT_SECRET", "secret")
	t.Setenv("MS_GRAPH_SENDER_EMAIL", "sender@example.com")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing NATS_URL")
	}
}

func TestLoad_MissingGraphCredentials(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:4222")
	// MS_GRAPH_MOCK_TOKEN is not set → credentials are required

	cases := []struct {
		name    string
		missing string
	}{
		{"no tenant", "MS_GRAPH_TENANT_ID"},
		{"no client id", "MS_GRAPH_CLIENT_ID"},
		{"no client secret", "MS_GRAPH_CLIENT_SECRET"},
		{"no sender email", "MS_GRAPH_SENDER_EMAIL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// set all required except tc.missing
			t.Setenv("MS_GRAPH_TENANT_ID", "tenant")
			t.Setenv("MS_GRAPH_CLIENT_ID", "client")
			t.Setenv("MS_GRAPH_CLIENT_SECRET", "secret")
			t.Setenv("MS_GRAPH_SENDER_EMAIL", "sender@example.com")
			t.Setenv(tc.missing, "")

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is missing", tc.missing)
			}
		})
	}
}

func TestLoad_MockTokenSkipsCredentialCheck(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("MS_GRAPH_MOCK_TOKEN", "dev-token")
	// deliberately leave Graph credentials unset

	cfg, err := Load()
	if err != nil {
		t.Fatalf("mock token must skip credential check, got: %v", err)
	}
	if cfg.GraphMockToken != "dev-token" {
		t.Errorf("GraphMockToken: want dev-token, got %s", cfg.GraphMockToken)
	}
}

func TestLoad_IntEnvOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DISPATCH_SPAM_TIMEOUT_SECONDS", "30")
	t.Setenv("DISPATCH_VALIDATION_MAX_BODY_SIZE", "5000000")
	t.Setenv("DISPATCH_MAX_TOTAL_ATTACHMENT_SIZE_MB", "10")
	t.Setenv("DISPATCH_NATS_PUBLISH_TIMEOUT_SECONDS", "3")
	t.Setenv("DISPATCH_GRAPH_RATE_LIMITER_SKIP_SLEEP", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SpamTimeoutSeconds != 30 {
		t.Errorf("SpamTimeoutSeconds: want 30, got %d", cfg.SpamTimeoutSeconds)
	}
	if cfg.MaxBodySize != 5_000_000 {
		t.Errorf("MaxBodySize: want 5000000, got %d", cfg.MaxBodySize)
	}
	if cfg.MaxTotalAttachmentMB != 10 {
		t.Errorf("MaxTotalAttachmentMB: want 10, got %d", cfg.MaxTotalAttachmentMB)
	}
	if cfg.NatsPublishTimeout != 3*time.Second {
		t.Errorf("NatsPublishTimeout: want 3s, got %v", cfg.NatsPublishTimeout)
	}
	if !cfg.GraphRateLimiterSkip {
		t.Error("GraphRateLimiterSkip: want true")
	}
}

func TestLoad_InvalidIntFallsToDefault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DISPATCH_SPAM_TIMEOUT_SECONDS", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SpamTimeoutSeconds != 60 {
		t.Errorf("expected default 60 on parse error, got %d", cfg.SpamTimeoutSeconds)
	}
}

func TestLoad_BounceMailboxDefaultsToSenderEmail(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MSGraphBounceMailbox != "sender@example.com" {
		t.Errorf("BounceMailbox: want sender@example.com, got %s", cfg.MSGraphBounceMailbox)
	}
}

func TestLoad_BounceMailboxOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MS_GRAPH_BOUNCE_MAILBOX", "bounces@example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MSGraphBounceMailbox != "bounces@example.com" {
		t.Errorf("BounceMailbox: want bounces@example.com, got %s", cfg.MSGraphBounceMailbox)
	}
}

func TestLoad_ProxyURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MS_GRAPH_PROXY_URL", "http://localhost:8000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GraphProxyURL != "http://localhost:8000" {
		t.Errorf("GraphProxyURL: want http://localhost:8000, got %s", cfg.GraphProxyURL)
	}
}

func TestLoad_MimeWhitelistOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DISPATCH_VALIDATION_MIME_WHITELIST", "image/png,text/plain")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.MimeWhitelist) != 2 {
		t.Fatalf("MimeWhitelist: want 2 entries, got %d", len(cfg.MimeWhitelist))
	}
	if cfg.MimeWhitelist[0] != "image/png" || cfg.MimeWhitelist[1] != "text/plain" {
		t.Errorf("MimeWhitelist: got %v", cfg.MimeWhitelist)
	}
}
