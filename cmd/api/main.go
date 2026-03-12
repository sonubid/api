// Package main is the entry point for the SonuBid API server. It wires
// together all internal packages and starts the HTTP/WebSocket server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/dto"
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

// bidProcessor is a narrow interface for the bid processing dependency used by
// wsHandler and makeMsgHandler. It allows both functions to be tested without
// importing the concrete processor package.
type bidProcessor interface {
	ProcessBid(ctx context.Context, bid auction.Bid, msg []byte) error
}

// Compile-time assertion that *processor.Processor satisfies bidProcessor.
var _ bidProcessor = (*processor.Processor)(nil)

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

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/auction/{auctionID}", wsHandler(h, proc, logger, os.Getenv("ALLOWED_ORIGIN")))

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

// wsHandler returns an http.HandlerFunc that upgrades the request to a
// WebSocket connection for the auction room identified by the {auctionID}
// path value and wires incoming messages to the processor.
// allowedOrigin is read once at startup and used for every connection.
func wsHandler(h *hub.Hub, proc bidProcessor, logger *slog.Logger, allowedOrigin string) http.HandlerFunc {
	opts := &websocket.AcceptOptions{
		OriginPatterns: []string{allowedOrigin},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		auctionID := r.PathValue("auctionID")

		msgHandler := makeMsgHandler(r.Context(), auctionID, proc, logger)

		hub.Handler(h, auctionID, msgHandler, opts)(w, r)
	}
}

// makeMsgHandler returns a closure that parses a raw WebSocket message as a
// BidRequest DTO, maps it to a domain Bid, and delegates to the bidProcessor.
// Malformed messages are logged and silently dropped. Rejected bids are logged
// at Warn level. The bid ID is generated from a nanosecond timestamp combined
// with auctionID and userID; collisions are possible under extreme concurrency
// and should be replaced with a UUID generator before production use.
func makeMsgHandler(ctx context.Context, auctionID string, proc bidProcessor, logger *slog.Logger) func([]byte) {
	return func(raw []byte) {
		var req dto.BidRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			logger.Warn("invalid bid message",
				slog.String("auction_id", auctionID),
				slog.Any("error", err))

			return
		}

		now := time.Now()

		bid := auction.Bid{
			ID:        fmt.Sprintf("%s-%s-%d", auctionID, req.UserID, now.UnixNano()),
			AuctionID: auctionID,
			UserID:    req.UserID,
			Amount:    req.Amount,
			PlacedAt:  now,
		}

		resp := dto.BidResponse{
			AuctionID: bid.AuctionID,
			UserID:    bid.UserID,
			Amount:    bid.Amount,
		}

		broadcastMsg, err := json.Marshal(resp)
		if err != nil {
			logger.Error("failed to marshal bid response",
				slog.String("auction_id", auctionID),
				slog.Any("error", err))

			return
		}

		if err := proc.ProcessBid(ctx, bid, broadcastMsg); err != nil {
			logger.Warn("bid rejected",
				slog.String("auction_id", auctionID),
				slog.String("user_id", req.UserID),
				slog.Uint64("amount", req.Amount),
				slog.Any("error", err))
		}
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
