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

// ---------------------------------------------------------------------------
// LoadState
// ---------------------------------------------------------------------------

func (s *storeSuite) TestLoadStateSuccess() {
	err := s.store.LoadState(s.ctx, s.newState(auctionOne, startingPrice))
	require.NoError(s.T(), err)

	// Verify state was created correctly
	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), auctionOne, state.AuctionID)
	require.Equal(s.T(), startingPrice, state.StartingPrice)
	require.Equal(s.T(), uint64(0), state.CurrentBid)
	require.Empty(s.T(), state.BidderID)
}

func (s *storeSuite) TestLoadStateEmptyAuctionID() {
	err := s.store.LoadState(s.ctx, s.newState("", startingPrice))
	require.ErrorIs(s.T(), err, auction.ErrAuctionNotFound)
}

func (s *storeSuite) TestLoadStateOverwrites() {
	err := s.store.LoadState(s.ctx, s.newState(auctionOne, startingPrice))
	require.NoError(s.T(), err)

	// Overwrite with different starting price
	newStartingPrice := uint64(200)
	err = s.store.LoadState(s.ctx, s.newState(auctionOne, newStartingPrice))
	require.NoError(s.T(), err)

	// Verify state was overwritten
	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), newStartingPrice, state.StartingPrice)
}

// ---------------------------------------------------------------------------
// GetState
// ---------------------------------------------------------------------------

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

	// Get state twice
	state1, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	state2, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)

	// Modify first copy
	state1.CurrentBid = 999999
	state1.BidderID = "hacker"

	// Second copy should be unchanged (proves we get copies, not references)
	require.Equal(s.T(), uint64(0), state2.CurrentBid)
	require.Empty(s.T(), state2.BidderID)
}

// ---------------------------------------------------------------------------
// TryUpdateBid
// ---------------------------------------------------------------------------

func (s *storeSuite) TestTryUpdateBidFirstBidSuccess() {
	s.loadAuction(auctionOne, startingPrice)

	bid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.NoError(s.T(), err)

	// Verify state updated
	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), higherBid, state.CurrentBid)
	require.Equal(s.T(), userOne, state.BidderID)
	require.Equal(s.T(), bid.PlacedAt, state.UpdatedAt)
}

func (s *storeSuite) TestTryUpdateBidSubsequentBidSuccess() {
	s.loadAuction(auctionOne, startingPrice)

	// Place first bid
	firstBid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, firstBid)
	require.NoError(s.T(), err)

	// Place second bid (higher)
	secondBid := s.newBid(auctionOne, userTwo, higherBid+50)
	err = s.store.TryUpdateBid(s.ctx, secondBid)
	require.NoError(s.T(), err)

	// Verify second bid won
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

	bid := s.newBid(auctionOne, userOne, lowerBid) // Below starting price
	err := s.store.TryUpdateBid(s.ctx, bid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

func (s *storeSuite) TestTryUpdateBidTooLowSubsequentBid() {
	s.loadAuction(auctionOne, startingPrice)

	// Place first bid
	firstBid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, firstBid)
	require.NoError(s.T(), err)

	// Try to place lower bid
	lowerBid := s.newBid(auctionOne, userTwo, higherBid-10)
	err = s.store.TryUpdateBid(s.ctx, lowerBid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

func (s *storeSuite) TestTryUpdateBidEqualBid() {
	s.loadAuction(auctionOne, startingPrice)

	// Place first bid
	firstBid := s.newBid(auctionOne, userOne, higherBid)
	err := s.store.TryUpdateBid(s.ctx, firstBid)
	require.NoError(s.T(), err)

	// Try to place equal bid (should fail)
	equalBid := s.newBid(auctionOne, userTwo, higherBid)
	err = s.store.TryUpdateBid(s.ctx, equalBid)
	require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
}

// ---------------------------------------------------------------------------
// Concurrent Access Tests
// ---------------------------------------------------------------------------

func (s *storeSuite) TestConcurrentReads() {
	s.loadAuction(auctionOne, startingPrice)

	var wg sync.WaitGroup
	errors := make(chan error, concurrentOps)

	// Start many concurrent readers
	for i := 0; i < concurrentOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.store.GetState(s.ctx, auctionOne)
			errors <- err
		}()
	}

	wg.Wait()
	close(errors)

	// All reads should succeed
	for err := range errors {
		require.NoError(s.T(), err)
	}
}

func (s *storeSuite) TestConcurrentBidsSameAmount() {
	s.loadAuction(auctionOne, startingPrice)

	var wg sync.WaitGroup
	results := make(chan error, concurrentOps)
	bidAmount := startingPrice + 50 // All bid the same amount

	// Start many concurrent bidders with the SAME bid amount
	for i := 0; i < concurrentOps; i++ {
		wg.Add(1)
		go func(bidderID int) {
			defer wg.Done()
			bid := s.newBidWithID(auctionOne, fmt.Sprintf("user-%d", bidderID), bidAmount)
			err := s.store.TryUpdateBid(s.ctx, bid)
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	// Count successes and failures
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
		}
	}

	// Only one bid should succeed when all bid the same amount
	require.Equal(s.T(), 1, successCount)

	// Verify final state has the bid amount
	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Equal(s.T(), bidAmount, state.CurrentBid)
}

func (s *storeSuite) TestConcurrentBidsIncreasing() {
	s.loadAuction(auctionOne, startingPrice)

	var wg sync.WaitGroup
	results := make(chan error, concurrentOps)

	// Start many concurrent bidders with increasing amounts
	for i := 0; i < concurrentOps; i++ {
		wg.Add(1)
		go func(bidAmount uint64, bidderID int) {
			defer wg.Done()
			bid := s.newBidWithID(auctionOne, fmt.Sprintf("user-%d", bidderID), bidAmount)
			err := s.store.TryUpdateBid(s.ctx, bid)
			results <- err
		}(startingPrice+uint64(i)+1, i) // Each bid higher than previous
	}

	wg.Wait()
	close(results)

	// Count successes and failures
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			require.ErrorIs(s.T(), err, auction.ErrBidTooLow)
		}
	}

	// Multiple bids should succeed (this is correct auction behavior!)
	require.Greater(s.T(), successCount, 1)
	require.Less(s.T(), successCount, concurrentOps) // But not all should succeed

	// Verify final state has a high bid (exact amount depends on timing)
	state, err := s.store.GetState(s.ctx, auctionOne)
	require.NoError(s.T(), err)
	require.Greater(s.T(), state.CurrentBid, startingPrice)
}

func (s *storeSuite) TestConcurrentLoadAndRead() {
	var wg sync.WaitGroup

	// Load auctions concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(auctionID string) {
			defer wg.Done()
			err := s.store.LoadState(s.ctx, s.newState(auctionID, startingPrice))
			require.NoError(s.T(), err)
		}(fmt.Sprintf("auction-%d", i))
	}

	wg.Wait()

	// Verify all auctions were created
	for i := 0; i < 10; i++ {
		state, err := s.store.GetState(s.ctx, fmt.Sprintf("auction-%d", i))
		require.NoError(s.T(), err)
		require.Equal(s.T(), startingPrice, state.StartingPrice)
	}
}

// ---------------------------------------------------------------------------
// Helper Methods
// ---------------------------------------------------------------------------

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

func (s *storeSuite) newBidWithID(auctionID, userID string, amount uint64) auction.Bid {
	s.T().Helper()
	return auction.Bid{
		ID:        fmt.Sprintf("bid-%s-%s-%d", userID, auctionID, time.Now().UnixNano()),
		AuctionID: auctionID,
		UserID:    userID,
		Amount:    amount,
		PlacedAt:  time.Now(),
	}
}
