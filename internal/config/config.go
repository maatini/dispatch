package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWorkerAckWaitSeconds = 300 // 5m
	defaultWorkerMaxDeliver     = 8
)

type Config struct {
	Port                 string
	NatsURL              string
	MSGraphTenantID      string
	MSGraphClientID      string
	MSGraphClientSecret  string
	MSGraphSenderEmail   string
	MSGraphBounceMailbox string
	SpamTimeoutSeconds   int
	MaxBodySize          int64
	MimeWhitelist        []string
	MaxTotalAttachmentMB int
	NatsPublishTimeout   time.Duration
	GraphRateLimiterSkip bool
	GraphProxyURL        string        // MS_GRAPH_PROXY_URL — routes Graph calls through Dev Proxy
	GraphMockToken       string        // MS_GRAPH_MOCK_TOKEN — skips OAuth2, makes Graph credentials optional
	AdminAuthSecret      string        // DISPATCH_ADMIN_AUTH_SECRET — HMAC secret for Admin-API JWT auth
	GatewayAuthToken     string        // DISPATCH_GATEWAY_AUTH_TOKEN — Bearer token for POST /mail/send
	GatewayAuthDisabled  bool          // DISPATCH_GATEWAY_AUTH_DISABLED=true — local dev only; skips send auth
	WorkerAckWait        time.Duration // DISPATCH_WORKER_ACK_WAIT_SECONDS — JetStream AckWait (default 5m)
	WorkerMaxDeliver     int           // DISPATCH_WORKER_MAX_DELIVER — finite redelivery limit (default 8; infinite forbidden)
}

func Load() (Config, error) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		return Config{}, fmt.Errorf("NATS_URL is required")
	}

	graphMockToken := os.Getenv("MS_GRAPH_MOCK_TOKEN")
	graphProxyURL := os.Getenv("MS_GRAPH_PROXY_URL")

	tenantID := os.Getenv("MS_GRAPH_TENANT_ID")
	clientID := os.Getenv("MS_GRAPH_CLIENT_ID")
	clientSecret := os.Getenv("MS_GRAPH_CLIENT_SECRET")
	senderEmail := os.Getenv("MS_GRAPH_SENDER_EMAIL")

	if graphMockToken == "" {
		if tenantID == "" {
			return Config{}, fmt.Errorf("MS_GRAPH_TENANT_ID is required")
		}
		if clientID == "" {
			return Config{}, fmt.Errorf("MS_GRAPH_CLIENT_ID is required")
		}
		if clientSecret == "" {
			return Config{}, fmt.Errorf("MS_GRAPH_CLIENT_SECRET is required")
		}
		if senderEmail == "" {
			return Config{}, fmt.Errorf("MS_GRAPH_SENDER_EMAIL is required")
		}
	}

	adminAuthSecret := os.Getenv("DISPATCH_ADMIN_AUTH_SECRET")

	// Gateway auth is enforced at mail-gateway startup (not here) so worker/admin/bounce
	// can share Load() without requiring a send token.
	gatewayAuthDisabled := os.Getenv("DISPATCH_GATEWAY_AUTH_DISABLED") == "true"
	gatewayAuthToken := os.Getenv("DISPATCH_GATEWAY_AUTH_TOKEN")

	bounceMailbox := os.Getenv("MS_GRAPH_BOUNCE_MAILBOX")
	if bounceMailbox == "" {
		bounceMailbox = envOr("MS_GRAPH_SENDER_EMAIL", "noreply@dev.local")
	}

	defaultMimeList := "application/pdf,image/jpeg,image/png,text/plain,application/msword," +
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	return Config{
		Port:                 envOr("PORT", "8080"),
		NatsURL:              natsURL,
		MSGraphTenantID:      tenantID,
		MSGraphClientID:      clientID,
		MSGraphClientSecret:  clientSecret,
		MSGraphSenderEmail:   senderEmail,
		MSGraphBounceMailbox: bounceMailbox,
		SpamTimeoutSeconds:   envInt("DISPATCH_SPAM_TIMEOUT_SECONDS", 60),
		MaxBodySize:          envInt64("DISPATCH_VALIDATION_MAX_BODY_SIZE", 10_000_000),
		MimeWhitelist:        strings.Split(envOr("DISPATCH_VALIDATION_MIME_WHITELIST", defaultMimeList), ","),
		MaxTotalAttachmentMB: envInt("DISPATCH_MAX_TOTAL_ATTACHMENT_SIZE_MB", 20),
		NatsPublishTimeout:   time.Duration(envInt("DISPATCH_NATS_PUBLISH_TIMEOUT_SECONDS", 5)) * time.Second,
		GraphRateLimiterSkip: os.Getenv("DISPATCH_GRAPH_RATE_LIMITER_SKIP_SLEEP") == "true",
		GraphProxyURL:        graphProxyURL,
		GraphMockToken:       graphMockToken,
		AdminAuthSecret:      adminAuthSecret,
		GatewayAuthToken:     gatewayAuthToken,
		GatewayAuthDisabled:  gatewayAuthDisabled,
		WorkerAckWait:        workerAckWait(),
		WorkerMaxDeliver:     workerMaxDeliver(),
	}, nil
}

// workerAckWait returns DISPATCH_WORKER_ACK_WAIT_SECONDS as a duration.
// Invalid or ≤0 values fall back to the 5m default (infinite/zero AckWait is not allowed).
func workerAckWait() time.Duration {
	secs := envIntPositive("DISPATCH_WORKER_ACK_WAIT_SECONDS", defaultWorkerAckWaitSeconds)
	return time.Duration(secs) * time.Second
}

// workerMaxDeliver returns DISPATCH_WORKER_MAX_DELIVER.
// Invalid or <1 values (including -1 / infinite) fall back to the default of 8.
func workerMaxDeliver() int {
	return envIntPositive("DISPATCH_WORKER_MAX_DELIVER", defaultWorkerMaxDeliver)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// envIntPositive is like envInt but rejects n < 1 (invalid parse, 0, and negative
// including -1 for infinite MaxDeliver) by returning def.
func envIntPositive(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func envInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}
