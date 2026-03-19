package auction

import "time"

// State represents the in-memory state of an auction at a given moment.
// It tracks the current highest bid, the bidder who placed it, the starting
// price, and when the state was last updated. This struct is used by the
// in-memory Store to validate and process new bids without hitting the
// database on every request.
type State struct {
	StartsAt      time.Time
	EndsAt        time.Time
	UpdatedAt     time.Time
	AuctionID     string
	BidderID      string
	Status        Status
	StartingPrice uint64
	CurrentBid    uint64
}
