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
	"dispatch/internal/msgraph"
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

	graphClient := msgraph.NewClient(cfg.MSGraphTenantID, cfg.MSGraphClientID, cfg.MSGraphClientSecret, cfg.GraphProxyURL, cfg.GraphMockToken)
	crawler := bounce.NewCrawler(msgraph.NewBounceService(graphClient), js, cfg.MSGraphBounceMailbox)

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
