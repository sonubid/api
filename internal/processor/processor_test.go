package processor_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/processor"
)

type processorSuite struct {
	suite.Suite

	ctx         context.Context
	store       *mockStore
	queue       *mockQueue
	broadcaster *mockBroadcaster
	proc        *processor.Processor
}

func TestProcessorSuite(t *testing.T) { suite.Run(t, new(processorSuite)) }

func (s *processorSuite) SetupTest() {
	s.ctx = context.Background()
	s.store = &mockStore{}
	s.queue = &mockQueue{}
	s.broadcaster = &mockBroadcaster{}
	s.proc = processor.New(s.store, s.queue, s.broadcaster, discardLogger())
}

func (s *processorSuite) TestNewWithNilLoggerDoesNotPanic() {
	proc := processor.New(s.store, s.queue, s.broadcaster, nil)

	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	err := proc.ProcessBid(s.ctx, bid, []byte(bidMsg))

	require.NoError(s.T(), err)
}

func (s *processorSuite) TestProcessBidEnqueuesAndBroadcastsOnSuccess() {
	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	err := s.proc.ProcessBid(s.ctx, bid, []byte(bidMsg))

	require.NoError(s.T(), err)
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == 1 }, waitTimeout, pollInterval)
	require.Equal(s.T(), bid, s.queue.firstEnqueueCall().Bid)
	require.Eventually(s.T(), func() bool { return s.broadcaster.broadcastCount() == 1 }, waitTimeout, pollInterval)
	require.Equal(s.T(), auctionOne, s.broadcaster.firstBroadcastCall().auctionID)
	require.Equal(s.T(), []byte(bidMsg), s.broadcaster.firstBroadcastCall().message)
}

func (s *processorSuite) TestProcessBidSetsEventReceivedAt() {
	before := time.Now()
	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	err := s.proc.ProcessBid(s.ctx, bid, []byte(bidMsg))

	after := time.Now()

	require.NoError(s.T(), err)
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == 1 }, waitTimeout, pollInterval)
	receivedAt := s.queue.firstEnqueueCall().ReceivedAt
	require.False(s.T(), receivedAt.Before(before), "ReceivedAt must not be before the call started")
	require.False(s.T(), receivedAt.After(after), "ReceivedAt must not be after the call returned")
}

func (s *processorSuite) TestProcessBidAlwaysEnqueuesOnBidTooLow() {
	s.store.tryUpdateBidFn = func(_ context.Context, _ auction.Bid) error {
		return auction.ErrBidTooLow
	}

	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    losingBid,
		PlacedAt:  time.Now(),
	}

	err := s.proc.ProcessBid(s.ctx, bid, []byte(losingBidMsg))

	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == 1 }, waitTimeout, pollInterval)
	require.Empty(s.T(), s.broadcaster.calls)
}

func (s *processorSuite) TestProcessBidAlwaysEnqueuesOnAuctionNotFound() {
	s.store.tryUpdateBidFn = func(_ context.Context, _ auction.Bid) error {
		return auction.ErrAuctionNotFound
	}

	bid := auction.Bid{
		ID:        bidID,
		AuctionID: ghostAuction,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	err := s.proc.ProcessBid(s.ctx, bid, []byte(bidMsg))

	require.ErrorIs(s.T(), err, auction.ErrAuctionNotFound)
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == 1 }, waitTimeout, pollInterval)
	require.Empty(s.T(), s.broadcaster.calls)
}

func (s *processorSuite) TestProcessBidAlwaysEnqueuesOnAuctionClosed() {
	s.store.tryUpdateBidFn = func(_ context.Context, _ auction.Bid) error {
		return auction.ErrAuctionClosed
	}

	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	err := s.proc.ProcessBid(s.ctx, bid, []byte(bidMsg))

	require.ErrorIs(s.T(), err, auction.ErrAuctionClosed)
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == 1 }, waitTimeout, pollInterval)
	require.Empty(s.T(), s.broadcaster.calls)
}

func (s *processorSuite) TestProcessBidContinuesWhenEnqueueErrors() {
	s.queue.enqueueFn = func(_ auction.BidEvent) error {
		return errors.New("queue is full")
	}

	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	err := s.proc.ProcessBid(s.ctx, bid, []byte(bidMsg))

	require.NoError(s.T(), err)
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == 1 }, waitTimeout, pollInterval)
	require.Len(s.T(), s.broadcaster.calls, 1)
}

func (s *processorSuite) TestProcessBidHandlesConcurrentCalls() {
	var wg sync.WaitGroup

	wg.Add(concurrentOps)

	bid := auction.Bid{
		ID:        bidID,
		AuctionID: auctionOne,
		UserID:    userOne,
		Amount:    winningBid,
		PlacedAt:  time.Now(),
	}

	for range concurrentOps {
		go func() {
			defer wg.Done()
			_ = s.proc.ProcessBid(s.ctx, bid, []byte(bidMsg))
		}()
	}

	wg.Wait()

	require.Equal(s.T(), concurrentOps, s.store.updateCallCount())
	require.Eventually(s.T(), func() bool { return s.queue.enqueueCount() == concurrentOps }, waitTimeout, pollInterval)
	require.Eventually(s.T(), func() bool { return s.broadcaster.broadcastCount() == concurrentOps }, waitTimeout, pollInterval)
}
