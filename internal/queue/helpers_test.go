package queue_test

import (
	"time"

	"github.com/sonubid/api/internal/auction"
)

func makeBidEvent(amount uint64) auction.BidEvent {
	return auction.BidEvent{
		Bid: auction.Bid{
			ID:        "bid-1",
			AuctionID: auctionID,
			UserID:    userID,
			Amount:    amount,
			PlacedAt:  time.Now(),
		},
		ReceivedAt: time.Now(),
	}
}
