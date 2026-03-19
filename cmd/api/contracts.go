package main

import (
	"context"
	"time"

	"github.com/sonubid/api/internal/auction"
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
