package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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
	GraphProxyURL        string // MS_GRAPH_PROXY_URL — routes Graph calls through Dev Proxy
	GraphMockToken       string // MS_GRAPH_MOCK_TOKEN — skips OAuth2, makes Graph credentials optional
	AdminAuthSecret      string // DISPATCH_ADMIN_AUTH_SECRET — HMAC secret for Admin-API JWT auth
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
	if adminAuthSecret == "" {
		return Config{}, fmt.Errorf("DISPATCH_ADMIN_AUTH_SECRET is required")
	}

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
	}, nil
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
