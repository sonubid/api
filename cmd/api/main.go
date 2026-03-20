// Package main is the entry point for the SonuBid API server.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := run(ctx, logger)
	if err != nil {
		logger.Error("server error", slog.Any("error", err))
		os.Exit(1) //nolint:gocritic // stop is deferred above; os.Exit is intentional after logging
	}
}
