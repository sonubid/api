package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sonubid/api/internal/auction"

	"github.com/sonubid/api/internal/worker"
)

type stateEvicter interface {
	DeleteState(ctx context.Context, auctionID string) error
}

type auctionFinalizer interface {
	FinishExpiredAuctions(ctx context.Context, now time.Time) error
	ListFinishedStates(ctx context.Context) ([]auction.State, error)
}

type queueCloser interface {
	Close()
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
