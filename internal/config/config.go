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
}

func Load() (Config, error) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		return Config{}, fmt.Errorf("NATS_URL is required")
	}
	tenantID := os.Getenv("MS_GRAPH_TENANT_ID")
	if tenantID == "" {
		return Config{}, fmt.Errorf("MS_GRAPH_TENANT_ID is required")
	}
	clientID := os.Getenv("MS_GRAPH_CLIENT_ID")
	if clientID == "" {
		return Config{}, fmt.Errorf("MS_GRAPH_CLIENT_ID is required")
	}
	clientSecret := os.Getenv("MS_GRAPH_CLIENT_SECRET")
	if clientSecret == "" {
		return Config{}, fmt.Errorf("MS_GRAPH_CLIENT_SECRET is required")
	}
	senderEmail := os.Getenv("MS_GRAPH_SENDER_EMAIL")
	if senderEmail == "" {
		return Config{}, fmt.Errorf("MS_GRAPH_SENDER_EMAIL is required")
	}

	bounceMailbox := os.Getenv("MS_GRAPH_BOUNCE_MAILBOX")
	if bounceMailbox == "" {
		bounceMailbox = senderEmail
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
		SpamTimeoutSeconds:   envInt("CODYMAIL_SPAM_TIMEOUT_SECONDS", 60),
		MaxBodySize:          envInt64("CODYMAIL_VALIDATION_MAX_BODY_SIZE", 10_000_000),
		MimeWhitelist:        strings.Split(envOr("CODYMAIL_VALIDATION_MIME_WHITELIST", defaultMimeList), ","),
		MaxTotalAttachmentMB: envInt("CODYMAIL_MAX_TOTAL_ATTACHMENT_SIZE_MB", 20),
		NatsPublishTimeout:   time.Duration(envInt("CODYMAIL_NATS_PUBLISH_TIMEOUT_SECONDS", 5)) * time.Second,
		GraphRateLimiterSkip: os.Getenv("CODYMAIL_GRAPH_RATE_LIMITER_SKIP_SLEEP") == "true",
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
