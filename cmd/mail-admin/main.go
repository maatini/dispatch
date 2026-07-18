package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/admin"
	"dispatch/internal/config"
	"dispatch/internal/httpsrv"
	"dispatch/internal/loggy"
	"dispatch/internal/natsutil"
	"dispatch/internal/sender"
	"dispatch/internal/version"
)

var log = loggy.GetLogger("mail-admin")

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Critical("config load", err)
		os.Exit(1)
	}
	if cfg.AdminAuthSecret == "" {
		log.Critical("config load", fmt.Errorf("DISPATCH_ADMIN_AUTH_SECRET is required"))
		os.Exit(1)
	}

	nc, js, err := natsutil.Connect(cfg.NatsURL)
	if err != nil {
		log.Critical("NATS connect", err)
		os.Exit(1)
	}
	defer nc.Close()

	spamTTL := time.Duration(cfg.SpamTimeoutSeconds) * time.Second
	if err := natsutil.Setup(js, spamTTL); err != nil {
		log.Critical("NATS setup failed", err)
		os.Exit(1)
	}

	sendersKV, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		log.Critical("senders KV", err)
		os.Exit(1)
	}

	senderStore := sender.New(sendersKV, sender.DefaultCacheTTL)
	resolver := admin.NewResolver(senderStore, js)

	handler, err := admin.NewHTTPHandler(resolver)
	if err != nil {
		log.Critical("graphql schema", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/graphql", admin.AuthMiddleware(cfg.AdminAuthSecret)(handler))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("mail-admin started", loggy.Kv("version", version.Version), loggy.Kv("port", cfg.Port))
	if err := httpsrv.Run(ctx, "mail-admin", ":"+cfg.Port, mux); err != nil {
		log.Critical("server error", err)
		os.Exit(1)
	}
	log.Info("mail-admin stopped")
}
