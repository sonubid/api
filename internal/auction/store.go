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
}
