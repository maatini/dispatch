package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/config"
	"dispatch/internal/loggy"
	"dispatch/internal/msgraph"
	"dispatch/internal/natsutil"
	"dispatch/internal/version"
	"dispatch/internal/worker"
)

var log = loggy.GetLogger("mail-worker")

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Critical("config load failed", err)
		os.Exit(1)
	}

	nc, js, err := natsutil.Connect(cfg.NatsURL)
	if err != nil {
		log.Critical("NATS connect failed", err)
		os.Exit(1)
	}
	defer nc.Close()

	spamTTL := time.Duration(cfg.SpamTimeoutSeconds) * time.Second
	if err := natsutil.Setup(js, spamTTL); err != nil {
		log.Critical("NATS setup failed", err)
		os.Exit(1)
	}
	if err := natsutil.ProvisionWorkerConsumer(js); err != nil {
		log.Critical("provision consumer", err)
		os.Exit(1)
	}
	objStore, err := natsutil.ProvisionObjectStore(js)
	if err != nil {
		log.Critical("provision object store failed", err)
		os.Exit(1)
	}

	deliveredKV, err := js.KeyValue(natsutil.BucketDelivered)
	if err != nil {
		log.Critical("delivered KV", err)
		os.Exit(1)
	}

	graphClient := msgraph.NewClient(cfg.MSGraphTenantID, cfg.MSGraphClientID, cfg.MSGraphClientSecret, cfg.GraphProxyURL, cfg.GraphMockToken)
	rateLimiter := msgraph.NewRateLimiter(cfg.GraphRateLimiterSkip)
	graphService := msgraph.NewService(graphClient, rateLimiter)

	attStore := worker.NewAttachmentStore(objStore)
	processor := worker.NewProcessor(graphService, deliveredKV, js, attStore)
	consumer := worker.NewConsumer(js, processor)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("mail-worker started", loggy.Kv("version", version.Version))
	if err := consumer.Run(ctx); err != nil {
		log.Critical("consumer error", err)
		os.Exit(1)
	}
	log.Info("mail-worker stopped")
}
