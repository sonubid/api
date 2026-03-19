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
	listenAddr                = ":8080"
	workersCount              = 10
	storeSyncIntervalDefault  = 5 * time.Second
	storeSyncIntervalEnvVar   = "STORE_SYNC_INTERVAL"
	auctionCleanupIntervalEnv = "AUCTION_CLEANUP_INTERVAL"
	storeSyncOperation        = "STORE_SYNC"
	auctionCleanupOperation   = "AUCTION_CLEANUP"
)

type stateLoader interface {
	LoadState(ctx context.Context, state auction.State) error
}

type stateLifecycleSyncer interface {
	SyncStateLifecycle(ctx context.Context, state auction.State) error
}

type stateEvicter interface {
	DeleteState(ctx context.Context, auctionID string) error
}

type activeStateProvider interface {
	ListActiveStates(ctx context.Context) ([]auction.State, error)
}

type auctionFinalizer interface {
	FinishExpiredAuctions(ctx context.Context, now time.Time) error
	ListFinishedStates(ctx context.Context) ([]auction.State, error)
}

type queueCloser interface {
	Close()
}

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
	auctionCleanupInterval, err := loadAuctionCleanupIntervalFromEnv(logger)
	if err != nil {
		return fmt.Errorf("load %s: %w", auctionCleanupIntervalEnv, err)
	}

	h := hub.NewHub()
	st := store.NewInMemory()
	q := queue.NewInMemory()
	proc := processor.New(logger, st, q, h)

	syncCtx, stopSync := context.WithCancel(ctx)
	defer stopSync()

	if err := seedStoreFromDB(ctx, logger, st, repo); err != nil {
		return fmt.Errorf("failed to seed store: %w", err)
	}

	wg := &sync.WaitGroup{}
	startWorkers(ctx, logger, wg, repo, q)
	startStoreSync(syncCtx, logger, wg, storeSyncInterval, st, repo)
	startAuctionCleanup(syncCtx, logger, wg, auctionCleanupInterval, st, repo)

	mux := handler.New(handler.Config{
		Hub:           h,
		Processor:     proc,
		Logger:        logger,
		AllowedOrigin: os.Getenv("ALLOWED_ORIGIN"),
	})

	srvErr := server.Start(ctx, logger, mux, listenAddr)
	stopSync()
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

// seedStoreFromDB loads every non-finished auction state from the repository
// into the in-memory store so the server is ready to accept bids on startup.
// If no active auctions exist the store starts empty without error.
func seedStoreFromDB(ctx context.Context, logger *slog.Logger, s stateLoader, provider activeStateProvider) error {
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

// startStoreSync launches a background ticker that periodically synchronises
// non-finished auctions from the repository into the in-memory store.
func startStoreSync(
	ctx context.Context,
	logger *slog.Logger,
	wg *sync.WaitGroup,
	interval time.Duration,
	s stateLifecycleSyncer,
	provider activeStateProvider,
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

// syncStoreFromDB synchronises non-finished auctions into the in-memory store,
// inserting missing entries and refreshing lifecycle fields for existing ones.
func syncStoreFromDB(ctx context.Context, logger *slog.Logger, s stateLifecycleSyncer, provider activeStateProvider) error {
	states, err := provider.ListActiveStates(ctx)
	if err != nil {
		return fmt.Errorf("list active states: %w", err)
	}

	for _, state := range states {
		if err := s.SyncStateLifecycle(ctx, state); err != nil {
			return fmt.Errorf("sync state lifecycle for auction %s: %w", state.AuctionID, err)
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

// startAuctionCleanup launches a background ticker that marks expired auctions
// as finished in the database, then evicts them from the in-memory store.
func startAuctionCleanup(
	ctx context.Context,
	logger *slog.Logger,
	wg *sync.WaitGroup,
	interval time.Duration,
	evicter stateEvicter,
	finalizer auctionFinalizer,
) {
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		logger.Info("auction cleanup worker started", slog.Duration("interval", interval))

		for {
			select {
			case <-ctx.Done():
				logger.Info("auction cleanup worker stopping")
				return
			case <-ticker.C:
				if err := cleanupFinishedAuctions(ctx, logger, evicter, finalizer, time.Now().UTC()); err != nil {
					logger.Error("auction cleanup failed", slog.Any("error", err))
				}
			}
		}
	})
}

// cleanupFinishedAuctions marks expired auctions as finished in persistent
// storage, then evicts those auction IDs from the in-memory store.
func cleanupFinishedAuctions(
	ctx context.Context,
	logger *slog.Logger,
	evicter stateEvicter,
	finalizer auctionFinalizer,
	now time.Time,
) error {
	if err := finalizer.FinishExpiredAuctions(ctx, now); err != nil {
		return fmt.Errorf("finish expired auctions: %w", err)
	}

	finishedStates, err := finalizer.ListFinishedStates(ctx)
	if err != nil {
		return fmt.Errorf("list finished states: %w", err)
	}

	var (
		evictedCount int
		evictErr     error
	)

	for _, state := range finishedStates {
		if err := evicter.DeleteState(ctx, state.AuctionID); err != nil {
			logger.Error(
				"failed to evict finished auction from store",
				slog.String("auction_id", state.AuctionID),
				slog.Any("error", err),
			)
			if evictErr == nil {
				evictErr = fmt.Errorf("evict auction %s: %w", state.AuctionID, err)
			}
			continue
		}
		evictedCount++
		logger.Debug("evicted finished auction from store", slog.String("auction_id", state.AuctionID))
	}

	if evictedCount > 0 {
		logger.Info("auction cleanup completed", slog.Int("finished_count", evictedCount))
	}

	if evictErr != nil {
		return evictErr
	}

	return nil
}

// startWorkers launches workersCount background workers in separate goroutines.
// Each worker drains the queue and persists bids via the provided Saver.
// The WaitGroup wg is incremented for each worker and decremented when it exits.
func startWorkers(ctx context.Context, logger *slog.Logger, wg *sync.WaitGroup, saver worker.Saver, q worker.Eventer) {
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
func shutdown(q queueCloser, wg *sync.WaitGroup, logger *slog.Logger) error {
	logger.Info("closing queue")
	q.Close()

	wg.Wait()

	logger.Info("shutdown complete")

	return nil
}
