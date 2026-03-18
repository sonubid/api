// Package store provides the in-memory auction state store used by the
// SonuBid API to validate and process incoming bids without database access.
package store

import (
	"context"
	"sync"
	"time"

	"github.com/sonubid/api/internal/auction"
)

// MemStore provides in-memory storage for auction states using a concurrent-safe
// map protected by a read-write mutex. It implements the auction.Store interface.
type MemStore struct {
	mu     sync.RWMutex
	states map[string]*auction.State
}

// Compile-time assertion that MemStore implements auction.Store.
var _ auction.Store = (*MemStore)(nil)

// New creates a new in-memory store with an empty state map.
func New() *MemStore {
	return &MemStore{
		states: make(map[string]*auction.State),
	}
}

// GetState retrieves the current state of an auction by its ID.
// Returns ErrAuctionNotFound if the auction does not exist.
func (s *MemStore) GetState(_ context.Context, auctionID string) (auction.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.states[auctionID]
	if !exists {
		return auction.State{}, auction.ErrAuctionNotFound
	}

	return *state, nil
}

// TryUpdateBid attempts to update the auction state with a new bid.
// It validates that the auction is active and that the bid amount is higher
// than the current bid and the starting price. Returns nil on success.
func (s *MemStore) TryUpdateBid(_ context.Context, bid auction.Bid) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.states[bid.AuctionID]
	if !exists {
		return auction.ErrAuctionNotFound
	}

	if err := s.validateBid(state, bid); err != nil {
		return err
	}

	state.CurrentBid = bid.Amount
	state.BidderID = bid.UserID
	state.UpdatedAt = bid.PlacedAt

	return nil
}

// LoadState initialises the in-memory state for a single auction.
// It is called during startup to seed the store from the Repository
// before the server begins accepting bids. If the auction already
// exists in the store, its state is replaced.
func (s *MemStore) LoadState(_ context.Context, state auction.State) error {
	if state.AuctionID == "" {
		return auction.ErrInvalidAuctionID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.AuctionID] = &state
	return nil
}

// LoadStateIfAbsent initialises the in-memory state for a single auction only
// when it does not already exist in the store.
func (s *MemStore) LoadStateIfAbsent(_ context.Context, state auction.State) error {
	if state.AuctionID == "" {
		return auction.ErrInvalidAuctionID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.states[state.AuctionID]; exists {
		return nil
	}

	s.states[state.AuctionID] = &state

	return nil
}

// validateBid checks that the auction is open at bid.PlacedAt and that
// bid.Amount is strictly greater than the minimum acceptable amount. The
// minimum is the starting price when no bids have been placed
// (CurrentBid == 0), or the current bid otherwise. When both CurrentBid and
// StartingPrice are zero, only bids with Amount > 0 succeed; a bid of zero is
// always rejected.
func (s *MemStore) validateBid(state *auction.State, bid auction.Bid) error {
	if !isAuctionOpenAt(state, bid.PlacedAt) {
		return auction.ErrAuctionClosed
	}
	var minBid uint64
	if state.CurrentBid == 0 {
		minBid = state.StartingPrice
	} else {
		minBid = state.CurrentBid
	}
	if bid.Amount <= minBid {
		return auction.ErrBidTooLow
	}
	return nil
}

// isAuctionOpenAt reports whether an auction can accept bids at t.
func isAuctionOpenAt(state *auction.State, t time.Time) bool {
	if state.Status == auction.StatusFinished {
		return false
	}
	if !state.StartsAt.IsZero() && t.Before(state.StartsAt) {
		return false
	}
	if !state.EndsAt.IsZero() && !t.Before(state.EndsAt) {
		return false
	}
	return true
}
