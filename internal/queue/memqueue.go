// Package queue provides an in-memory queue backed by a buffered Go channel.
package queue

import (
	"sync"

	"github.com/sonubid/api/internal/auction"
)

// defaultQueueSize is the capacity of the underlying buffered channel.
const defaultQueueSize = 100

// MemQueue is a non-blocking, in-memory queue backed by a buffered channel.
// It is safe for concurrent use.
// Close must be called exactly once after all Enqueue calls have finished.
type MemQueue struct {
	once   sync.Once
	events chan auction.BidEvent
}

// NewInMemory creates a MemQueue with a buffer capacity of defaultQueueSize.
func NewInMemory() *MemQueue {
	return &MemQueue{
		events: make(chan auction.BidEvent, defaultQueueSize),
	}
}

// Close shuts down the queue by closing the underlying events channel.
// It is safe to call Close multiple times; only the first call closes the channel.
// Callers must ensure that Enqueue is not called concurrently with or after Close.
func (mq *MemQueue) Close() {
	mq.once.Do(func() { close(mq.events) })
}

// Enqueue adds the bid event to the queue without blocking.
// Returns ErrQueueFull if the internal buffer is at capacity.
func (mq *MemQueue) Enqueue(event auction.BidEvent) error {
	select {
	case mq.events <- event:
		return nil
	default:
		return ErrQueueFull
	}
}

// Events returns a read-only channel that emits bid events in the order they
// were enqueued. The channel is closed when Close is called.
func (mq *MemQueue) Events() <-chan auction.BidEvent {
	return mq.events
}
