package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codymail-go/internal/config"
	"codymail-go/internal/gateway"
	"codymail-go/internal/natsutil"
	"codymail-go/internal/quota"
	"codymail-go/internal/sender"
	"codymail-go/internal/spam"
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
		slog.Error("provision streams failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := natsutil.ProvisionKVBuckets(js, spamTTL); err != nil {
		slog.Error("provision KV buckets failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	objStore, err := natsutil.ProvisionObjectStore(js)
	if err != nil {
		slog.Error("provision object store failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	sendersKV, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		slog.Error("senders KV", slog.String("error", err.Error()))
		os.Exit(1)
	}
	quotaKV, err := js.KeyValue(natsutil.BucketQuota)
	if err != nil {
		slog.Error("quota KV", slog.String("error", err.Error()))
		os.Exit(1)
	}
	spamKV, err := js.KeyValue(natsutil.BucketSpam)
	if err != nil {
		slog.Error("spam KV", slog.String("error", err.Error()))
		os.Exit(1)
	}

	senderStore := sender.New(sendersKV, 10*time.Minute)
	quotaChecker := quota.NewChecker(quotaKV)
	spamChecker := spam.NewChecker(spamKV)
	publisher := gateway.NewNatsPublisher(js, cfg.NatsPublishTimeout)
	attStore := gateway.NewAttachmentStore(objStore)

	handler := gateway.NewHandler(cfg, senderStore, quotaChecker, spamChecker, publisher, attStore)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("mail-gateway started", slog.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", slog.String("error", err.Error()))
	}
}
