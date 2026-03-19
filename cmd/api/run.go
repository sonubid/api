package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/sonubid/api/internal/db"
	"github.com/sonubid/api/internal/handler"
	"github.com/sonubid/api/internal/hub"
	"github.com/sonubid/api/internal/processor"
	"github.com/sonubid/api/internal/queue"
	"github.com/sonubid/api/internal/repository"
	"github.com/sonubid/api/internal/server"
	"github.com/sonubid/api/internal/store"
)

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
