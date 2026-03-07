package auction

import "context"

// Repository defines the contract for persisting bids to a database.
// Implementations handle the actual storage logic, whether to PostgreSQL,
// MySQL, or any other backing store.
type Repository interface {
	// Save persists a bid to the underlying storage.
	// Returns an error if the save operation fails.
	Save(ctx context.Context, bid Bid) error
}
