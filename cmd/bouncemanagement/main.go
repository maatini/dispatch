package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/bounce"
	"dispatch/internal/config"
	"dispatch/internal/loggy"
	"dispatch/internal/msgraph"
	"dispatch/internal/natsutil"
	"dispatch/internal/version"
)

var log = loggy.GetLogger("bouncemanagement")

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Critical("config load", err)
		os.Exit(1)
	}

	nc, js, err := natsutil.Connect(cfg.NatsURL)
	if err != nil {
		log.Critical("NATS connect", err)
		os.Exit(1)
	}
	defer nc.Close()

	spamTTL := time.Duration(cfg.SpamTimeoutSeconds) * time.Second
	if err := natsutil.ProvisionStreams(js); err != nil {
		log.Critical("provision streams", err)
		os.Exit(1)
	}
	if err := natsutil.ProvisionKVBuckets(js, spamTTL); err != nil {
		log.Critical("provision KV", err)
		os.Exit(1)
	}

	graphClient := msgraph.NewClient(cfg.MSGraphTenantID, cfg.MSGraphClientID, cfg.MSGraphClientSecret, cfg.GraphProxyURL, cfg.GraphMockToken)
	crawler := bounce.NewCrawler(msgraph.NewBounceService(graphClient), js, cfg.MSGraphBounceMailbox)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	log.Info("bouncemanagement started", loggy.Kv("version", version.Version), loggy.Kv("mailbox", cfg.MSGraphBounceMailbox))

	// run immediately on start
	if err := crawler.Run(ctx); err != nil {
		log.Error("crawler error", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("bouncemanagement stopped")
			return
		case <-ticker.C:
			if err := crawler.Run(ctx); err != nil {
				log.Error("crawler error", err)
			}
		}
	}
}
