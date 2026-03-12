package repository_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/repository"
)

const (
	auctionOne     = "auction-1"
	userOne        = "user-1"
	userTwo        = "user-2"
	bidAmount      = uint64(100)
	bidAmountStep  = uint64(50)
	mutatedAmount  = uint64(9999)
	concurrentSave = 50
)

type repositorySuite struct {
	suite.Suite

	ctx  context.Context
	repo *repository.MemRepository
}

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(repositorySuite))
}

func (s *repositorySuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = repository.NewMemRepository()
}

func (s *repositorySuite) TestSaveStoredBidMatchesInput() {
	bid := newBid("bid-1", auctionOne, userOne, bidAmount)

	err := s.repo.Save(s.ctx, bid)

	require.NoError(s.T(), err)
	saved := s.repo.Saved()
	require.Len(s.T(), saved, 1)
	require.Equal(s.T(), bid, saved[0])
}

func (s *repositorySuite) TestSaveAccumulatesMultipleBids() {
	bidOne := newBid("bid-1", auctionOne, userOne, bidAmount)
	bidTwo := newBid("bid-2", auctionOne, userTwo, bidAmount+bidAmountStep)

	require.NoError(s.T(), s.repo.Save(s.ctx, bidOne))
	require.NoError(s.T(), s.repo.Save(s.ctx, bidTwo))

	saved := s.repo.Saved()
	require.Len(s.T(), saved, 2)
	require.Equal(s.T(), bidOne, saved[0])
	require.Equal(s.T(), bidTwo, saved[1])
}

func (s *repositorySuite) TestSavePreservesOrder() {
	const count = 5

	for i := range count {
		bid := newBid(bidIDFromIndex(i), auctionOne, userOne, bidAmount+uint64(i)) //nolint:gosec // i is bounded by count (5), no overflow
		require.NoError(s.T(), s.repo.Save(s.ctx, bid))
	}

	saved := s.repo.Saved()
	require.Len(s.T(), saved, count)

	for i := range count {
		require.Equal(s.T(), bidAmount+uint64(i), saved[i].Amount) //nolint:gosec // i is bounded by count (5), no overflow
	}
}

func (s *repositorySuite) TestSavedReturnsCopyNotReference() {
	bid := newBid("bid-1", auctionOne, userOne, bidAmount)
	require.NoError(s.T(), s.repo.Save(s.ctx, bid))

	snapshot := s.repo.Saved()
	snapshot[0].Amount = mutatedAmount

	fresh := s.repo.Saved()
	require.Equal(s.T(), bidAmount, fresh[0].Amount)
}

func (s *repositorySuite) TestSaveConcurrentCallsAreRaceSafe() {
	var wg sync.WaitGroup

	for i := range concurrentSave {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bid := newBid(bidIDFromIndex(n), auctionOne, userOne, bidAmount+uint64(n)) //nolint:gosec // n is bounded by concurrentSave (50), no overflow
			_ = s.repo.Save(s.ctx, bid)                                                // MemRepository.Save always returns nil
		}(i)
	}

	wg.Wait()

	require.Len(s.T(), s.repo.Saved(), concurrentSave)
}

// newBid is a test factory for auction.Bid.
func newBid(id, auctionID, userID string, amount uint64) auction.Bid {
	return auction.Bid{
		ID:        id,
		AuctionID: auctionID,
		UserID:    userID,
		Amount:    amount,
		PlacedAt:  time.Now(),
	}
}

// bidIDFromIndex returns a deterministic bid ID for the given index.
func bidIDFromIndex(i int) string {
	return "bid-" + strconv.Itoa(i)
}
