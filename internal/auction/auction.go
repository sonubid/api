// Package auction provides the domain models and interfaces for the auction system.
//
// This package defines the core entities (Auction, Bid, State, BidEvent) and the
// contracts (Store, Repository) that enable the application to operate
// with different storage and persistence implementations.
package auction

import "time"

// Status represents the current state of an auction.
type Status string

const (
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusFinished Status = "finished"
)

// Auction represents an auction entity with its metadata.
// It contains the auction's unique identifier, title, current status,
// starting price, and scheduling information.
type Auction struct {
	ID            string
	Title         string
	Status        Status
	StartingPrice uint64
	StartsAt      time.Time
	EndsAt        time.Time
}
