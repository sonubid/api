package repository

import (
	"context"
	"sync"

	"github.com/sonubid/api/internal/auction"
)

// MemRepository is an in-memory bid persistence implementation.
// It is intended for MVP and testing purposes only; it does not persist data
// across process restarts.
type MemRepository struct {
	mu   sync.RWMutex
	data []auction.Bid
}

// NewMemRepository returns a new, empty MemRepository.
func NewMemRepository() *MemRepository {
	return &MemRepository{}
}

// Save appends bid to the in-memory store.
func (r *MemRepository) Save(_ context.Context, bid auction.Bid) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = append(r.data, bid)

	return nil
}

// Saved returns a copy of all bids stored so far.
// It is provided for testing and debugging; it is not part of the
// worker.Saver contract.
func (r *MemRepository) Saved() []auction.Bid {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]auction.Bid, len(r.data))
	copy(result, r.data)

	return result
}
