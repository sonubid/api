// Package worker provides a background goroutine that drains the Queue and
// persists bids via a Saver.
package worker

import (
	"context"
	"log/slog"

	"github.com/sonubid/api/internal/auction"
)

// Saver defines the minimal persistence contract required by Worker.
type Saver interface {
	Save(ctx context.Context, bid auction.Bid) error
}

// Eventer defines the minimal queue contract required by Worker.
type Eventer interface {
	Events() <-chan auction.BidEvent
}

// Worker drains bid events from a Queue and persists each one via a Saver.
// It is safe to run multiple Workers concurrently against the same Queue.
type Worker struct {
	saver  Saver
	queue  Eventer
	logger *slog.Logger
}

// New constructs a Worker wired to the given Saver, Queue, and logger.
// If logger is nil, slog.Default() is used.
func New(saver Saver, queue Eventer, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}

	return &Worker{
		saver:  saver,
		queue:  queue,
		logger: logger,
	}
}

// Start begins processing bid events from the Queue until either the context
// is cancelled or the Queue channel is closed. It blocks the calling goroutine
// and is intended to be launched with go w.Start(ctx, workerID).
//
// Shutdown ordering: when ctx is cancelled and the Queue channel still has
// buffered events, Go's select picks a branch at random. To guarantee that
// every in-flight event is persisted, callers must stop all producers and
// close the Queue before cancelling ctx.
func (w *Worker) Start(ctx context.Context, workerID int) {
	w.logger.Info("worker started", slog.Int("worker_id", workerID))

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker shutting down", slog.Int("worker_id", workerID))
			return
		case event, ok := <-w.queue.Events():
			if !ok {
				w.logger.Info("queue closed, worker exiting", slog.Int("worker_id", workerID))
				return
			}
			if err := w.saver.Save(ctx, event.Bid); err != nil {
				w.logger.Error("failed to save bid",
					slog.Any("error", err),
					slog.Int("worker_id", workerID))
			} else {
				w.logger.Info("bid saved successfully",
					slog.String("auction_id", event.Bid.AuctionID),
					slog.String("user_id", event.Bid.UserID),
					slog.Uint64("amount", event.Bid.Amount),
					slog.Int("worker_id", workerID))
			}
		}
	}
}
