// Package server provides a lifecycle-managed HTTP server that starts on demand
// and shuts down gracefully when the provided context is cancelled.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// readHeaderTimeout is the maximum duration allowed to read request headers.
// It guards against Slowloris attacks during the initial HTTP/WebSocket upgrade.
// ReadTimeout, WriteTimeout, and IdleTimeout are intentionally left unset because
// WebSocket connections are long-lived: a global read/write deadline would
// terminate active WebSocket sessions after the timeout expires.
const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 3 * time.Second
)

// Start creates and starts an HTTP server bound to addr, using handler for all
// requests.
//
// It blocks until ctx is cancelled or the server encounters a fatal
// error.
//
// On context cancellation it performs a graceful shutdown and returns nil.
// On any other error (e.g. address already in use) it returns the error immediately.
//
// Returns ErrNilLogger, ErrInvalidAddress, or ErrNilHandler when the corresponding
// argument is invalid.
func Start(ctx context.Context, logger *slog.Logger, handler http.Handler, addr string) error {
	if addr == "" {
		return ErrInvalidAddress
	}
	if logger == nil {
		return ErrNilLogger
	}
	if handler == nil {
		return ErrNilHandler
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)
		logger.Info("server listening", slog.String("addr", addr))

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, shutting down HTTP server")
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		return shutdown(shutCtx, srv, logger)
	}
}

// shutdown performs a graceful HTTP server shutdown bounded by ctx.
func shutdown(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	logger.Info("HTTP server shutdown complete")
	return nil
}
