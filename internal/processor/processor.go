// Package processor validates incoming bids, updates the in-memory auction
// state on success, and enqueues bid events for asynchronous persistence for
// outcomes that should be audited.
package processor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sonubid/api/internal/auction"
)

type Updater interface {
	TryUpdateBid(ctx context.Context, bid auction.Bid) error
}

// Enqueuer defines the minimal queue contract required by Processor.
type Enqueuer interface {
	Enqueue(event auction.BidEvent) error
}

// Broadcaster defines the contract for broadcasting messages to all clients
// connected to a given auction room. Implementations must be safe for
// concurrent use.
type Broadcaster interface {
	// Broadcast sends a message to every client currently registered in the
	// auction identified by auctionID. Clients that cannot receive the
	// message immediately are skipped to avoid blocking the caller.
	Broadcast(auctionID string, message []byte)
}

// Processor serialises store validation so that only one bid update runs at a
// time, then asynchronously enqueues the event for audit persistence and
// broadcasts the result to connected clients.
// All methods are safe for concurrent use.
type Processor struct {
	mu          sync.Mutex
	logger      *slog.Logger
	store       Updater
	queue       Enqueuer
	broadcaster Broadcaster
}

// New creates a Processor wired to the given store, queue, broadcaster, and
// logger. If logger is nil, slog.Default() is used.
func New(logger *slog.Logger, store Updater, queue Enqueuer, broadcaster Broadcaster) *Processor {
	if logger == nil {
		logger = slog.Default()
	}

	return &Processor{
		store:       store,
		queue:       queue,
		broadcaster: broadcaster,
		logger:      logger,
	}
}

// ProcessBid validates and processes an incoming bid.
// The bid event is enqueued asynchronously for audit persistence except when
// the auction is closed, so queue errors never block the real-time bidding
// path. If the bid becomes the new highest bid the raw message is broadcast to
// all clients in the auction room. Returns an error only if the bid does not
// pass store validation.
func (p *Processor) ProcessBid(ctx context.Context, bid auction.Bid, msg []byte) error {
	receivedAt := time.Now()

	p.mu.Lock()
	updateErr := p.store.TryUpdateBid(ctx, bid)
	p.mu.Unlock()

	if !errors.Is(updateErr, auction.ErrAuctionClosed) {
		go p.enqueueAsync(auction.BidEvent{
			Bid:        bid,
			ReceivedAt: receivedAt,
		})
	}
	if updateErr != nil {
		return fmt.Errorf("processor: %w", updateErr)
	}

	p.broadcaster.Broadcast(bid.AuctionID, msg)

	return nil
}

// enqueueAsync submits the bid event to the queue and logs any failure.
func (p *Processor) enqueueAsync(event auction.BidEvent) {
	if err := p.queue.Enqueue(event); err != nil {
		p.logger.Error("processor: failed to enqueue bid event",
			slog.String("auction_id", event.Bid.AuctionID),
			slog.Any("error", err))
	}
}
