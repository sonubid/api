package auction

import "context"

// Repository defines the contract for persisting bids to a database.
// Implementations handle the actual storage logic, whether to PostgreSQL,
// MySQL, or any other backing store.
type Repository interface {
	// Save persists a bid to the underlying storage.
	// Returns an error if the save operation fails.
	Save(ctx context.Context, bid Bid) error

	// ListActiveStates returns the current state for every auction that is
	// not yet finished. It is used during startup to seed the in-memory
	// Store before the server begins accepting bids.
	ListActiveStates(ctx context.Context) ([]State, error)
}
