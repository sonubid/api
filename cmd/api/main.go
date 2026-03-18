// Package main is the entry point for the SonuBid API server. It wires
// together all internal packages and starts the HTTP/WebSocket server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/db"
	"github.com/sonubid/api/internal/handler"
	"github.com/sonubid/api/internal/hub"
	"github.com/sonubid/api/internal/processor"
	"github.com/sonubid/api/internal/queue"
	"github.com/sonubid/api/internal/repository"
	"github.com/sonubid/api/internal/server"
	"github.com/sonubid/api/internal/store"
	"github.com/sonubid/api/internal/worker"
)

// Server configuration constants.
const (
	listenAddr               = ":8080"
	workersCount             = 10
	storeSyncIntervalDefault = 5 * time.Second
	storeSyncIntervalEnvVar  = "STORE_SYNC_INTERVAL"
)

// Compile-time assertion that *processor.Processor satisfies handler.BidProcessor.
var _ handler.BidProcessor = (*processor.Processor)(nil)

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

// run wires all components, seeds the in-memory store from PostgreSQL, starts
// background workers, and serves HTTP until ctx is cancelled.
func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL environment variable is not set")
	}

	migrateDSN := strings.Replace(dsn, "postgres://", "pgx5://", 1)
	migrateDSN = strings.Replace(migrateDSN, "postgresql://", "pgx5://", 1)
	if err := db.RunMigrations(migrateDSN); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()
	repo := repository.NewPostgresRepository(pool)

	storeSyncInterval, err := loadStoreSyncIntervalFromEnv(logger)
	if err != nil {
		return fmt.Errorf("load %s: %w", storeSyncIntervalEnvVar, err)
	}

	h := hub.NewHub()
	st := store.New()
	q := queue.New()
	proc := processor.New(st, q, h, logger)

	syncCtx, stopStoreSync := context.WithCancel(ctx)
	defer stopStoreSync()

	if err := seedStoreFromDB(ctx, logger, st, repo); err != nil {
		return fmt.Errorf("failed to seed store: %w", err)
	}

	wg := &sync.WaitGroup{}
	startWorkers(ctx, logger, wg, repo, q)
	startStoreSync(syncCtx, logger, wg, storeSyncInterval, st, repo)

	mux := handler.New(handler.Config{
		Hub:           h,
		Processor:     proc,
		Logger:        logger,
		AllowedOrigin: os.Getenv("ALLOWED_ORIGIN"),
	})

	srvErr := server.Start(ctx, logger, mux, listenAddr)
	stopStoreSync()
	shutErr := shutdown(q, wg, logger)

	if srvErr != nil {
		return srvErr
	}

	return shutErr
}

// loadStoreSyncIntervalFromEnv returns the background store sync interval.
// When STORE_SYNC_INTERVAL is empty, a safe default interval is used.
func loadStoreSyncIntervalFromEnv(logger *slog.Logger) (time.Duration, error) {
	raw := os.Getenv(storeSyncIntervalEnvVar)
	if raw == "" {
		logger.Info("using default store sync interval", slog.Duration("interval", storeSyncIntervalDefault))
		return storeSyncIntervalDefault, nil
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

// seedStoreFromDB loads every non-finished auction state from the repository
// into the in-memory store so the server is ready to accept bids on startup.
// If no active auctions exist the store starts empty without error.
func seedStoreFromDB(ctx context.Context, logger *slog.Logger, s auction.Store, provider auction.ActiveStateProvider) error {
	states, err := provider.ListActiveStates(ctx)
	if err != nil {
		return fmt.Errorf("list active states: %w", err)
	}

	for _, state := range states {
		logger.Debug(
			"seeding store",
			slog.String("auction_id", state.AuctionID),
			slog.String("status", string(state.Status)),
			slog.Uint64("starting_price", state.StartingPrice),
			slog.String("bidder_id", state.BidderID),
			slog.Uint64("current_bid", state.CurrentBid),
			slog.Time("updated_at", state.UpdatedAt))
		if err := s.LoadState(ctx, state); err != nil {
			return fmt.Errorf("load state for auction %s: %w", state.AuctionID, err)
		}
	}

	return nil
}

// startStoreSync launches a background ticker that periodically syncs newly
// created non-finished auctions from the repository into the in-memory store.
func startStoreSync(
	ctx context.Context,
	logger *slog.Logger,
	wg *sync.WaitGroup,
	interval time.Duration,
	s auction.StateSyncLoader,
	provider auction.ActiveStateProvider,
) {
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		logger.Info("store sync worker started", slog.Duration("interval", interval))

		for {
			select {
			case <-ctx.Done():
				logger.Info("store sync worker stopping")
				return
			case <-ticker.C:
				if err := syncStoreFromDB(ctx, logger, s, provider); err != nil {
					logger.Error("store sync failed", slog.Any("error", err))
				}
			}
		}
	})
}

// syncStoreFromDB loads non-finished auctions into the in-memory store only
// when they are not already present.
func syncStoreFromDB(ctx context.Context, logger *slog.Logger, s auction.StateSyncLoader, provider auction.ActiveStateProvider) error {
	states, err := provider.ListActiveStates(ctx)
	if err != nil {
		return fmt.Errorf("list active states: %w", err)
	}

	for _, state := range states {
		if err := s.LoadStateIfAbsent(ctx, state); err != nil {
			return fmt.Errorf("load state if absent for auction %s: %w", state.AuctionID, err)
		}
		logger.Debug(
			"store sync tick",
			slog.String("auction_id", state.AuctionID),
			slog.String("status", string(state.Status)),
			slog.Time("starts_at", state.StartsAt),
			slog.Time("ends_at", state.EndsAt),
		)
	}

	return nil
}

// startWorkers launches workersCount background workers in separate goroutines.
// Each worker drains the queue and persists bids via the provided Saver.
// The WaitGroup wg is incremented for each worker and decremented when it exits.
func startWorkers(ctx context.Context, logger *slog.Logger, wg *sync.WaitGroup, saver auction.Saver, q auction.Queue) {
	for i := 1; i <= workersCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w := worker.New(saver, q, logger)
			w.Start(ctx, workerID)
		}(i)
	}
}

// shutdown closes the queue so workers drain their remaining events,
// then waits for all workers to exit before returning.
// Order: close queue → workers drain → return.
func shutdown(q auction.Queue, wg *sync.WaitGroup, logger *slog.Logger) error {
	logger.Info("closing queue")
	q.Close()

	wg.Wait()

	logger.Info("shutdown complete")

	return nil
}
