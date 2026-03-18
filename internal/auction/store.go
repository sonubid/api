package auction

import "context"

// Store defines the contract for managing auction state in memory.
// It provides methods to retrieve the current state of an auction and to
// attempt updating it with a new bid. Implementations must be safe for
// concurrent access, typically using a mutex or similar synchronization.
type Store interface {
	// GetState retrieves the current state of an auction by its ID.
	// Returns ErrAuctionNotFound if the auction does not exist.
	GetState(ctx context.Context, auctionID string) (State, error)

	// TryUpdateBid attempts to update the auction state with a new bid.
	// It validates that the bid amount is higher than the current bid and
	// the starting price. Returns nil on success, or an error indicating
	// why the update failed (ErrBidTooLow, ErrAuctionClosed, etc.).
	TryUpdateBid(ctx context.Context, bid Bid) error

	// LoadState initialises the in-memory state for a single auction.
	// It is called during startup to seed the store from the Repository
	// before the server begins accepting bids. If the auction already
	// exists in the store, its state is replaced.
	LoadState(ctx context.Context, state State) error
}

// StateSyncLoader defines the contract used by the background store sync loop.
// Implementations insert state only when the auction is not already present.
type StateSyncLoader interface {
	// LoadStateIfAbsent initialises state only when the auction is missing.
	LoadStateIfAbsent(ctx context.Context, state State) error
}
