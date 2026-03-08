package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/store"
)

const (
	auctionOne    = "auction-1"
	ghostAuction  = "ghost-auction"
	startingPrice = uint64(100)
	higherBid     = uint64(150)
	lowerBid      = uint64(50)
	userOne       = "user-1"
	userTwo       = "user-2"
	concurrentOps = 100
)

type storeSuite struct {
	suite.Suite

	ctx   context.Context
	store *store.MemStore
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, new(storeSuite))
}

func (s *storeSuite) SetupTest() {
	s.ctx = context.Background()
	s.store = store.New()
}

func (s *storeSuite) TestLoadStateSuccess() {
	err := s.store.LoadState(s.ctx, s.newState(auctionOne, startingPrice))
	require.NoError(s.T(), err)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), auctionOne, state.AuctionID)
	require.Equal(s.T(), startingPrice, state.StartingPrice)
	require.Equal(s.T(), uint64(0), state.CurrentBid)
	require.Empty(s.T(), state.BidderID)
}

func (s *storeSuite) TestLoadStateEmptyAuctionID() {
	err := s.store.LoadState(s.ctx, s.newState("", startingPrice))
	require.ErrorIs(s.T(), err, auction.ErrInvalidAuctionID)
}

func (s *storeSuite) TestLoadStateOverwrites() {
	err := s.store.LoadState(s.ctx, s.newState(auctionOne, startingPrice))
	require.NoError(s.T(), err)

	newStartingPrice := uint64(200)
	err = s.store.LoadState(s.ctx, s.newState(auctionOne, newStartingPrice))
	require.NoError(s.T(), err)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), newStartingPrice, state.StartingPrice)
}

func (s *storeSuite) TestGetStateSuccess() {
	s.loadAuction(auctionOne, startingPrice)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), auctionOne, state.AuctionID)
	require.Equal(s.T(), startingPrice, state.StartingPrice)
}

func (s *storeSuite) TestGetStateNotFound() {
	state, err := s.store.GetState(s.ctx, ghostAuction)
	require.ErrorIs(s.T(), err, auction.ErrAuctionNotFound)
	require.Equal(s.T(), auction.State{}, state)
}

func (s *storeSuite) TestGetStateReturnsCopy() {
	s.loadAuction(auctionOne, startingPrice)

	state1, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)

	state1.CurrentBid = 9999
	state1.BidderID = "mutated"

	state2, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)

	require.Equal(s.T(), uint64(0), state2.CurrentBid)
	require.Empty(s.T(), state2.BidderID)
}

func (s *storeSuite) TestTryUpdateBidFirstBidSuccess() {
	s.loadAuction(auctionOne, startingPrice)

	bid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.NoError(s.T(), err)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), higherBid, state.CurrentBid)
	require.Equal(s.T(), userOne, state.BidderID)
	require.Equal(s.T(), bid.PlacedAt, state.UpdatedAt)
}

func (s *storeSuite) TestTryUpdateBidSubsequentBidSuccess() {
	s.loadAuction(auctionOne, startingPrice)

	firstBid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, firstBid)
	require.NoError(s.T(), err)

	secondBid := s.newBid(auctionOne, userTwo, higherBid+50)
	err = s.store.TryUpdateBid(s.ctx, secondBid)
	require.NoError(s.T(), err)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), higherBid+50, state.CurrentBid)
	require.Equal(s.T(), userTwo, state.BidderID)
}

func (s *storeSuite) TestTryUpdateBidAuctionNotFound() {
	bid := s.newBid(ghostAuction, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.ErrorIs(s.T(), err, auction.ErrAuctionNotFound)
}

func (s *storeSuite) TestTryUpdateBidTooLowFirstBid() {
	s.loadAuction(auctionOne, startingPrice)

	bid := s.newBid(auctionOne, userOne, lowerBid)
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

func (s *storeSuite) TestTryUpdateBidZeroStartingPriceRejectsZeroAmount() {
	s.loadAuction(auctionOne, 0)

	bid := s.newBid(auctionOne, userOne, 0)
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

func (s *storeSuite) TestTryUpdateBidZeroStartingPriceAcceptsPositiveAmount() {
	s.loadAuction(auctionOne, 0)

	bid := s.newBid(auctionOne, userOne, 1)
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.NoError(s.T(), err)
}

func (s *storeSuite) TestTryUpdateBidTooLowSubsequentBid() {
	s.loadAuction(auctionOne, startingPrice)

	firstBid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, firstBid)
	require.NoError(s.T(), err)

	lowerBid := s.newBid(auctionOne, userTwo, higherBid-10)
	err = s.store.TryUpdateBid(s.ctx, lowerBid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

func (s *storeSuite) TestTryUpdateBidEqualBid() {
	s.loadAuction(auctionOne, startingPrice)

	firstBid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, firstBid)
	require.NoError(s.T(), err)

	equalBid := s.newBid(auctionOne, userTwo, higherBid)
	err = s.store.TryUpdateBid(s.ctx, equalBid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

func (s *storeSuite) TestConcurrentReads() {
	s.loadAuction(auctionOne, startingPrice)

	var wg sync.WaitGroup
	errors := make(chan error, concurrentOps)

	for range concurrentOps {
		wg.Go(func() {
			_, err := s.store.GetState(s.ctx, auctionOne)
			errors <- err
		})
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		require.NoError(s.T(), err)
	}
}

func (s *storeSuite) TestConcurrentBidsSameAmount() {
	s.loadAuction(auctionOne, startingPrice)

	var wg sync.WaitGroup
	results := make(chan error, concurrentOps)
	bidAmount := startingPrice + 50

	for i := range concurrentOps {
		wg.Go(func() {
			bid := s.newUniqueBid(auctionOne, fmt.Sprintf("user-%d", i), bidAmount)
			err := s.store.TryUpdateBid(s.ctx, bid)
			results <- err
		})
	}

	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
		}
	}

	require.Equal(s.T(), 1, successCount)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), bidAmount, state.CurrentBid)
}

func (s *storeSuite) TestConcurrentBidsIncreasing() {
	s.loadAuction(auctionOne, startingPrice)

	var wg sync.WaitGroup
	results := make(chan error, concurrentOps)

	for i := range concurrentOps {
		wg.Go(func() {
			bidAmount := startingPrice + uint64(i) + 1 //nolint:gosec // i is bounded by concurrentOps (100), no overflow risk
			bid := s.newUniqueBid(auctionOne, fmt.Sprintf("user-%d", i), bidAmount)
			err := s.store.TryUpdateBid(s.ctx, bid)
			results <- err
		})
	}

	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
		}
	}

	// Both bounds are probabilistic: with 100 goroutines submitting strictly
	// increasing bids concurrently, the scheduler is expected to interleave
	// them so that more than one succeeds and fewer than all succeed. This
	// assertion may fail under extreme serialization on heavily loaded CI
	// machines, but is reliable in practice.
	require.Greater(s.T(), successCount, 1)
	require.Less(s.T(), successCount, concurrentOps)

	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Greater(s.T(), state.CurrentBid, startingPrice)
}

func (s *storeSuite) TestConcurrentLoadAndRead() {
	var wg sync.WaitGroup

	for i := range 10 {
		wg.Go(func() {
			err := s.store.LoadState(s.ctx, s.newState(fmt.Sprintf("auction-%d", i), startingPrice))
			require.NoError(s.T(), err)
		})
	}

	wg.Wait()

	for i := range 10 {
		state, err := s.store.GetState(s.ctx, fmt.Sprintf("auction-%d", i))
		require.NoError(s.T(), err)
		require.Equal(s.T(), startingPrice, state.StartingPrice)
	}
}

func (s *storeSuite) loadAuction(auctionID string, startingPrice uint64) {
	s.T().Helper()
	err := s.store.LoadState(s.ctx, s.newState(auctionID, startingPrice))
	require.NoError(s.T(), err)
}

func (s *storeSuite) newState(auctionID string, startingPrice uint64) auction.State {
	s.T().Helper()
	return auction.State{
		AuctionID:     auctionID,
		StartingPrice: startingPrice,
	}
}

func (s *storeSuite) newBid(auctionID, userID string, amount uint64) auction.Bid {
	s.T().Helper()
	return auction.Bid{
		ID:        "bid-" + userID + "-" + auctionID,
		AuctionID: auctionID,
		UserID:    userID,
		Amount:    amount,
		PlacedAt:  time.Now(),
	}
}

func (s *storeSuite) newUniqueBid(auctionID, userID string, amount uint64) auction.Bid {
	s.T().Helper()
	return auction.Bid{
		ID:        fmt.Sprintf("bid-%s-%s-%d", userID, auctionID, time.Now().UnixNano()),
		AuctionID: auctionID,
		UserID:    userID,
		Amount:    amount,
		PlacedAt:  time.Now(),
	}
}
