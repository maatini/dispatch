package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/admin"
	"dispatch/internal/config"
	"dispatch/internal/natsutil"
	"dispatch/internal/sender"
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

	sendersKV, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		slog.Error("senders KV", slog.String("error", err.Error()))
		os.Exit(1)
	}

	senderStore := sender.New(sendersKV, 10*time.Minute)
	resolver := admin.NewResolver(senderStore, js)

	handler, err := admin.NewHTTPHandler(resolver)
	if err != nil {
		slog.Error("graphql schema", slog.String("error", err.Error()))
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
		slog.Info("mail-admin started", slog.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
