package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Server configuration constants.
const (
	listenAddr                = ":8080"
	workersCount              = 10
	storeSyncIntervalDefault  = 5 * time.Second
	storeSyncIntervalEnvVar   = "STORE_SYNC_INTERVAL"
	auctionCleanupIntervalEnv = "AUCTION_CLEANUP_INTERVAL"
	storeSyncOperation        = "STORE_SYNC"
	auctionCleanupOperation   = "AUCTION_CLEANUP"
)

// loadStoreSyncIntervalFromEnv returns the background store sync interval.
// When STORE_SYNC_INTERVAL is empty, a safe default interval is used.
func loadStoreSyncIntervalFromEnv(logger *slog.Logger) (time.Duration, error) {
	raw := os.Getenv(storeSyncIntervalEnvVar)
	return loadPositiveDurationFromEnv(raw, storeSyncIntervalDefault, logger, storeSyncOperation)
}

// loadAuctionCleanupIntervalFromEnv returns the background auction cleanup interval.
// When AUCTION_CLEANUP_INTERVAL is empty, the store sync interval default is used.
func loadAuctionCleanupIntervalFromEnv(logger *slog.Logger) (time.Duration, error) {
	raw := os.Getenv(auctionCleanupIntervalEnv)
	return loadPositiveDurationFromEnv(raw, storeSyncIntervalDefault, logger, auctionCleanupOperation)
}

// loadPositiveDurationFromEnv parses raw as a strictly positive duration, or
// returns defaultValue when raw is empty.
func loadPositiveDurationFromEnv(raw string, defaultValue time.Duration, logger *slog.Logger, operation string) (time.Duration, error) {
	if raw == "" {
		logger.Info(
			"using default background interval",
			slog.String("operation", operation),
			slog.Duration("interval", defaultValue),
		)
		return defaultValue, nil
	}

	interval, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	if interval <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}

	return interval, nil
}
