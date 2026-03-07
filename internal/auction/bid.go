package auction

import "time"

// Bid represents a single bid placed by a user on an auction.
// It is an immutable record of a user's offer amount at a specific point in time.
type Bid struct {
	ID        string
	AuctionID string
	UserID    string
	Amount    uint64
	PlacedAt  time.Time
}
