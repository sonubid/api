package processor_test

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sonubid/api/internal/auction"
)

const (
	auctionOne    = "auction-1"
	ghostAuction  = "ghost-auction"
	bidID         = "bid-1"
	userOne       = "user-1"
	winningBid    = uint64(200)
	losingBid     = uint64(50)
	bidMsg        = `{"bid":200}`
	losingBidMsg  = `{"bid":50}`
	concurrentOps = 50
	waitTimeout   = 2 * time.Second
	pollInterval  = 10 * time.Millisecond
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

type mockStore struct {
	mu             sync.Mutex
	tryUpdateBidFn func(ctx context.Context, bid auction.Bid) error
	tryUpdateCalls []auction.Bid
}

func (m *mockStore) GetState(_ context.Context, _ string) (auction.State, error) {
	return auction.State{}, nil
}

func (m *mockStore) TryUpdateBid(ctx context.Context, bid auction.Bid) error {
	m.mu.Lock()
	m.tryUpdateCalls = append(m.tryUpdateCalls, bid)
	m.mu.Unlock()

	if m.tryUpdateBidFn != nil {
		return m.tryUpdateBidFn(ctx, bid)
	}

	return nil
}

func (m *mockStore) LoadState(_ context.Context, _ auction.State) error {
	return nil
}

func (m *mockStore) updateCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.tryUpdateCalls)
}

type mockQueue struct {
	mu           sync.Mutex
	enqueueFn    func(event auction.BidEvent) error
	enqueueCalls []auction.BidEvent
}

func (m *mockQueue) Enqueue(event auction.BidEvent) error {
	m.mu.Lock()
	m.enqueueCalls = append(m.enqueueCalls, event)
	m.mu.Unlock()

	if m.enqueueFn != nil {
		return m.enqueueFn(event)
	}

	return nil
}

func (m *mockQueue) Events() <-chan auction.BidEvent {
	ch := make(chan auction.BidEvent)
	close(ch)

	return ch
}

func (m *mockQueue) enqueueCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.enqueueCalls)
}

func (m *mockQueue) firstEnqueueCall() auction.BidEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.enqueueCalls[0]
}

type broadcastCall struct {
	auctionID string
	message   []byte
}

type mockBroadcaster struct {
	mu    sync.Mutex
	calls []broadcastCall
}

func (m *mockBroadcaster) Broadcast(auctionID string, message []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, broadcastCall{auctionID: auctionID, message: message})
}

func (m *mockBroadcaster) broadcastCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.calls)
}

func (m *mockBroadcaster) firstBroadcastCall() broadcastCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.calls[0]
}
