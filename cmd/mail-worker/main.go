package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codymail-go/internal/config"
	"codymail-go/internal/msgraph"
	"codymail-go/internal/natsutil"
	"codymail-go/internal/worker"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	nc, js, err := natsutil.Connect(cfg.NatsURL)
	if err != nil {
		slog.Error("NATS connect failed", slog.String("error", err.Error()))
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
	if err := natsutil.ProvisionWorkerConsumer(js); err != nil {
		slog.Error("provision consumer", slog.String("error", err.Error()))
		os.Exit(1)
	}

	deliveredKV, err := js.KeyValue(natsutil.BucketDelivered)
	if err != nil {
		slog.Error("delivered KV", slog.String("error", err.Error()))
		os.Exit(1)
	}

	graphClient := msgraph.NewClient(cfg.MSGraphTenantID, cfg.MSGraphClientID, cfg.MSGraphClientSecret)
	rateLimiter := msgraph.NewRateLimiter(cfg.GraphRateLimiterSkip)
	graphService := msgraph.NewService(graphClient, rateLimiter)

	processor := worker.NewProcessor(graphService, deliveredKV, js)
	consumer := worker.NewConsumer(js, processor)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("mail-worker started")
	if err := consumer.Run(ctx); err != nil {
		slog.Error("consumer error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("mail-worker stopped")
}
