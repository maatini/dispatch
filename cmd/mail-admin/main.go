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
	if err := natsutil.ProvisionStreams(js); err != nil {
		log.Critical("provision streams", err)
		os.Exit(1)
	}
	if err := natsutil.ProvisionKVBuckets(js, spamTTL); err != nil {
		log.Critical("provision KV", err)
		os.Exit(1)
	}

	sendersKV, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		log.Critical("senders KV", err)
		os.Exit(1)
	}

	senderStore := sender.New(sendersKV, 10*time.Minute)
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

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("mail-admin started", loggy.Kv("version", version.Version), loggy.Kv("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Critical("server error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
