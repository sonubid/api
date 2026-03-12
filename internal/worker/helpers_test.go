package worker_test

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sonubid/api/internal/auction"
)

const (
	auctionOne  = "auction-1"
	userOne     = "user-1"
	userTwo     = "user-2"
	bidAmount   = uint64(100)
	step        = uint64(50)
	workerID    = 1
	workerCount = 3
	eventCount  = 10
	waitTimeout = 2 * time.Second
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func makeBidEvent(auctionID, userID string, amount uint64) auction.BidEvent {
	return auction.BidEvent{
		Bid: auction.Bid{
			AuctionID: auctionID,
			UserID:    userID,
			Amount:    amount,
			PlacedAt:  time.Now(),
		},
		ReceivedAt: time.Now(),
	}
}

// mockRepository is a test double for auction.Saver.
type mockRepository struct {
	mu        sync.Mutex
	saveFn    func(ctx context.Context, bid auction.Bid) error
	saveCalls []auction.Bid
}

func (m *mockRepository) Save(ctx context.Context, bid auction.Bid) error {
	m.mu.Lock()
	m.saveCalls = append(m.saveCalls, bid)
	m.mu.Unlock() // released before saveFn so slow test functions do not hold the lock

	if m.saveFn != nil {
		return m.saveFn(ctx, bid)
	}

	return nil
}

func (m *mockRepository) saveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.saveCalls)
}

func (m *mockRepository) firstSaveCall() auction.Bid {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.saveCalls) == 0 {
		panic("firstSaveCall: no calls recorded")
	}

	return m.saveCalls[0]
}

// mockQueue is a test double for auction.Queue backed by a real buffered channel.
type mockQueue struct {
	once   sync.Once
	events chan auction.BidEvent
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		events: make(chan auction.BidEvent, eventCount*2),
	}
}

func (m *mockQueue) Enqueue(event auction.BidEvent) error {
	m.events <- event
	return nil
}

func (m *mockQueue) Events() <-chan auction.BidEvent {
	return m.events
}

func (m *mockQueue) Close() {
	m.once.Do(func() { close(m.events) })
}

func (m *mockQueue) send(event auction.BidEvent) {
	m.events <- event
}
