// Package repository provides implementations of the auction.Repository interface.
package repository

import (
	"context"
	"sync"

	"github.com/sonubid/api/internal/auction"
)

// MemRepository is an in-memory implementation of auction.Repository.
// It is intended for MVP and testing purposes only; it does not persist data
// across process restarts.
type MemRepository struct {
	mu   sync.RWMutex
	data []auction.Bid
}

var _ auction.Repository = (*MemRepository)(nil)

// NewMemRepository returns a new, empty MemRepository.
func NewMemRepository() *MemRepository {
	return &MemRepository{}
}

// Save appends bid to the in-memory store. It always returns nil.
func (r *MemRepository) Save(_ context.Context, bid auction.Bid) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = append(r.data, bid)

	return nil
}

// ListActiveStates always returns nil, nil.
// This method is not implemented in the in-memory repository.
func (r *MemRepository) ListActiveStates(_ context.Context) ([]auction.State, error) {
	return nil, nil
}

// Saved returns a copy of all bids stored so far.
// It is provided for testing and debugging; it is not part of the
// auction.Repository interface.
func (r *MemRepository) Saved() []auction.Bid {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]auction.Bid, len(r.data))
	copy(result, r.data)

	return result
}
