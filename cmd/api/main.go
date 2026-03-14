// Package main is the entry point for the SonuBid API server. It wires
// together all internal packages and starts the HTTP/WebSocket server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sonubid/api/internal/auction"
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
	listenAddr        = ":8080"
	seedAuctionID     = "auction-1"
	seedStartingPrice = uint64(1000)
	workersCount      = 10
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

// run wires all components, seeds initial state, starts background workers,
// and serves HTTP until ctx is cancelled.
func run(ctx context.Context, logger *slog.Logger) error {
	h := hub.NewHub()
	st := store.New()
	q := queue.New()
	repo := repository.NewMemRepository()
	proc := processor.New(st, q, h, logger)

	if err := seedStore(ctx, st); err != nil {
		return fmt.Errorf("failed to seed store: %w", err)
	}

	wg := &sync.WaitGroup{}
	startWorkers(ctx, logger, wg, repo, q)

	mux := handler.New(handler.Config{
		Hub:           h,
		Processor:     proc,
		Logger:        logger,
		AllowedOrigin: os.Getenv("ALLOWED_ORIGIN"),
	})

	srvErr := server.Start(ctx, logger, mux, listenAddr)
	shutErr := shutdown(q, wg, logger)

	if srvErr != nil {
		return srvErr
	}
	return shutErr
}

// seedStore loads an initial auction state so the server is ready to accept
// bids on startup without a database round-trip.
func seedStore(ctx context.Context, s *store.MemStore) error {
	return s.LoadState(ctx, auction.State{
		AuctionID:     seedAuctionID,
		StartingPrice: seedStartingPrice,
	})
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
