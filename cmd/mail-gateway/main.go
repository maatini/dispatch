package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/config"
	"dispatch/internal/gateway"
	"dispatch/internal/loggy"
	"dispatch/internal/natsutil"
	"dispatch/internal/quota"
	"dispatch/internal/sender"
	"dispatch/internal/spam"
	"dispatch/internal/version"
)

var log = loggy.GetLogger("mail-gateway")

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
	if err := natsutil.ProvisionStreams(js); err != nil {
		log.Critical("provision streams failed", err)
		os.Exit(1)
	}
	if err := natsutil.ProvisionKVBuckets(js, spamTTL); err != nil {
		log.Critical("provision KV buckets failed", err)
		os.Exit(1)
	}
	objStore, err := natsutil.ProvisionObjectStore(js)
	if err != nil {
		log.Critical("provision object store failed", err)
		os.Exit(1)
	}

	sendersKV, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		log.Critical("senders KV", err)
		os.Exit(1)
	}
	quotaKV, err := js.KeyValue(natsutil.BucketQuota)
	if err != nil {
		log.Critical("quota KV", err)
		os.Exit(1)
	}
	spamKV, err := js.KeyValue(natsutil.BucketSpam)
	if err != nil {
		log.Critical("spam KV", err)
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
		log.Info("mail-gateway started", loggy.Kv("version", version.Version), loggy.Kv("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Critical("server error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Critical("shutdown error", err)
	}
}
