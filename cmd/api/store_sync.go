package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sonubid/api/internal/auction"
)

type stateLoader interface {
	LoadState(ctx context.Context, state auction.State) error
}

type stateLifecycleSyncer interface {
	SyncStateLifecycle(ctx context.Context, state auction.State) error
}

type activeStateProvider interface {
	ListActiveStates(ctx context.Context) ([]auction.State, error)
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
