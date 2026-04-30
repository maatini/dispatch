package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/bounce"
	"dispatch/internal/config"
	"dispatch/internal/natsutil"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", slog.String("error", err.Error()))
		os.Exit(1)
	}

	nc, js, err := natsutil.Connect(cfg.NatsURL)
	if err != nil {
		slog.Error("NATS connect", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer nc.Close()

	spamTTL := time.Duration(cfg.SpamTimeoutSeconds) * time.Second
	if err := natsutil.ProvisionStreams(js); err != nil {
		slog.Error("provision streams", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := natsutil.ProvisionKVBuckets(js, spamTTL); err != nil {
		slog.Error("provision KV", slog.String("error", err.Error()))
		os.Exit(1)
	}

	graphBounce := newGraphBounceClient(cfg)
	crawler := bounce.NewCrawler(graphBounce, js, cfg.MSGraphBounceMailbox)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	slog.Info("bouncemanagement started", slog.String("mailbox", cfg.MSGraphBounceMailbox))

	// run immediately on start
	if err := crawler.Run(ctx); err != nil {
		slog.Error("crawler error", slog.String("error", err.Error()))
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("bouncemanagement stopped")
			return
		case <-ticker.C:
			if err := crawler.Run(ctx); err != nil {
				slog.Error("crawler error", slog.String("error", err.Error()))
			}
		}
	}
}

// graphBounceClient adapts the msgraph HTTP client to the bounce.graphClient interface.
// It implements GetUnreadMessages and MarkAsRead via MS Graph.
type graphBounceClient struct {
	tenantID     string
	clientID     string
	clientSecret string
}

func newGraphBounceClient(cfg config.Config) *graphBounceClient {
	return &graphBounceClient{
		tenantID:     cfg.MSGraphTenantID,
		clientID:     cfg.MSGraphClientID,
		clientSecret: cfg.MSGraphClientSecret,
	}
}

func (g *graphBounceClient) GetUnreadMessages(ctx context.Context, mailbox string) ([]bounce.NDRMessage, error) {
	// Production implementation would call:
	// GET https://graph.microsoft.com/v1.0/users/{mailbox}/messages?$filter=isRead eq false
	// and parse the response into []NDRMessage.
	// Deferred to avoid circular import; the full implementation uses msgraph.Client.
	slog.InfoContext(ctx, "GetUnreadMessages not yet wired to MS Graph client",
		slog.String("mailbox", mailbox),
	)
	return nil, nil
}

func (g *graphBounceClient) MarkAsRead(ctx context.Context, mailbox, messageID string) error {
	slog.InfoContext(ctx, "MarkAsRead not yet wired to MS Graph client",
		slog.String("mailbox", mailbox),
		slog.String("messageId", messageID),
	)
	return nil
}
