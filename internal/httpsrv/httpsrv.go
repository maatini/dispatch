// Package httpsrv provides the shared HTTP server lifecycle for the dispatch services.
package httpsrv

import (
	"context"
	"errors"
	"net/http"
	"time"

	"dispatch/internal/loggy"
)

const shutdownTimeout = 10 * time.Second

// Run serves HTTP on addr until ctx is cancelled, then shuts down gracefully.
// A listen/serve failure is returned; shutdown errors are logged, not returned.
func Run(ctx context.Context, name, addr string, handler http.Handler) error {
	log := loggy.GetLogger(name)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", loggy.Kv("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
