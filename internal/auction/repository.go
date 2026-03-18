package auction

import (
	"context"
	"time"
)

// Saver defines the contract for persisting a single bid to storage.
type Saver interface {
	// Save persists bid to the underlying storage and returns an error if the operation fails.
	Save(ctx context.Context, bid Bid) error
}

// ActiveStateProvider defines the contract for listing the current state of
// every auction that is not yet finished. It is used during startup to seed
// the in-memory Store before the server begins accepting bids.
type ActiveStateProvider interface {
	// ListActiveStates returns the state for every non-finished auction.
	ListActiveStates(ctx context.Context) ([]State, error)
}

// Finalizer defines the contract for transitioning expired auctions to
// finished in persistent storage.
type Finalizer interface {
	// FinishExpiredAuctions marks every non-finished auction with EndsAt <= now
	// as finished.
	FinishExpiredAuctions(ctx context.Context, now time.Time) error

	// ListFinishedStates returns the state for every finished auction. It is used
	// by background cleanup workers to evict stale in-memory entries that might
	// remain after transient cache failures.
	ListFinishedStates(ctx context.Context) ([]State, error)
}

// Repository combines Saver and ActiveStateProvider into a single interface.
// Full storage implementations (e.g. PostgreSQL) satisfy Repository; lightweight
// implementations may satisfy only one of the smaller interfaces.
type Repository interface {
	Saver
	ActiveStateProvider
	Finalizer
}
