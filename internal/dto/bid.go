// Package dto defines the Data Transfer Objects used for WebSocket message
// serialisation and deserialisation. DTOs decouple the wire format from the
// domain model, allowing both to evolve independently.
package dto

import "time"

// BidRequest is the JSON payload sent by a browser client to place a bid.
// AuctionID is inferred from the WebSocket URL path and is not part of this payload.
type BidRequest struct {
	// UserID identifies the bidder.
	UserID string `json:"user_id"`
	// Amount is the bid value in cents.
	Amount uint64 `json:"amount"`
}

// BidResponse is the JSON payload broadcast to all clients in an auction room
// when a bid is accepted. It carries the server-assigned ID and timestamp
// alongside the bidder and amount fields.
type BidResponse struct {
	// ID is the server-assigned unique identifier for the bid.
	ID string `json:"id"`
	// AuctionID is the identifier of the auction this bid belongs to.
	AuctionID string `json:"auction_id"`
	// UserID identifies the bidder.
	UserID string `json:"user_id"`
	// Amount is the bid value in cents.
	Amount uint64 `json:"amount"`
	// PlacedAt is the server-side timestamp when the bid was received.
	PlacedAt time.Time `json:"placed_at"`
}
