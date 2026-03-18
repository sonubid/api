// Package repository_test contains integration tests for the repository package.
package repository_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/db"
	"github.com/sonubid/api/internal/repository"
)

const (
	testDBImage    = "postgres:18.3-alpine3.23"
	testDBName     = "sonubid_test"
	testDBUser     = "sonubid"
	testDBPassword = "sonubid"
	sslModeDisable = "sslmode=disable"
	userID         = "user-1"
	UserID2        = "user-2"

	containerStartupTimeout = 60 * time.Second

	testStartingPrice uint64 = 100
	testBidAmountZero uint64 = 0
	testBidAmountLow  uint64 = 150
	testBidAmountMid  uint64 = 200
	testBidAmountHigh uint64 = 300
	testBidAmountSolo uint64 = 500

	concurrentSavers = 10
	concurrentBase   = 100
)

// postgresRepositorySuite is the testify suite for PostgresRepository integration tests.
// Each test method runs against a real PostgreSQL container managed by testcontainers-go.
type postgresRepositorySuite struct {
	suite.Suite

	container *tcpostgres.PostgresContainer
	pool      *pgxpool.Pool
	repo      *repository.PostgresRepository
}

// TestPostgresRepositorySuite is the suite runner.
func TestPostgresRepositorySuite(t *testing.T) {
	suite.Run(t, new(postgresRepositorySuite))
}

// SetupSuite starts the PostgreSQL container and runs migrations once for the
// entire suite. The container is terminated in TearDownSuite.
func (s *postgresRepositorySuite) SetupSuite() {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, testDBImage,
		tcpostgres.WithDatabase(testDBName),
		tcpostgres.WithUsername(testDBUser),
		tcpostgres.WithPassword(testDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(containerStartupTimeout),
		),
	)
	s.Require().NoError(err)

	s.container = container

	connStr, err := container.ConnectionString(ctx, sslModeDisable)
	s.Require().NoError(err)

	// golang-migrate pgx/v5 driver requires the pgx5:// scheme.
	migrateDSN := strings.Replace(connStr, "postgres://", "pgx5://", 1)
	migrateDSN = strings.Replace(migrateDSN, "postgresql://", "pgx5://", 1)
	s.Require().NoError(db.RunMigrations(migrateDSN))

	pool, err := db.Connect(ctx, connStr)
	s.Require().NoError(err)

	s.pool = pool
	s.repo = repository.NewPostgresRepository(pool)
}

// TearDownSuite closes the pool and terminates the container.
func (s *postgresRepositorySuite) TearDownSuite() {
	s.pool.Close()
	_ = s.container.Terminate(context.Background())
}

// SetupTest truncates both tables before each test for isolation.
func (s *postgresRepositorySuite) SetupTest() {
	_, err := s.pool.Exec(context.Background(), "TRUNCATE TABLE bid, auction RESTART IDENTITY CASCADE")
	s.Require().NoError(err)
}

// TestSavePersistsBid verifies that Save writes a bid row to the database.
func (s *postgresRepositorySuite) TestSavePersistsBid() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusActive)

	bid := auction.Bid{
		ID:        uuid.NewString(),
		AuctionID: auctionID,
		UserID:    userID,
		Amount:    testBidAmountMid,
		PlacedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	err := s.repo.Save(context.Background(), bid)
	s.Require().NoError(err)

	var count int
	err = s.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM bid WHERE id = $1", bid.ID).Scan(&count)
	s.Require().NoError(err)
	s.Equal(1, count)
}

// TestSaveReturnsErrorOnDuplicateID verifies that saving a bid with an
// already-existing ID returns a wrapped error.
func (s *postgresRepositorySuite) TestSaveReturnsErrorOnDuplicateID() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusActive)

	bid := auction.Bid{
		ID:        uuid.NewString(),
		AuctionID: auctionID,
		UserID:    userID,
		Amount:    testBidAmountMid,
		PlacedAt:  time.Now().UTC(),
	}

	s.Require().NoError(s.repo.Save(context.Background(), bid))

	err := s.repo.Save(context.Background(), bid)
	s.Require().Error(err)
	s.ErrorContains(err, "repository: save bid")
}

// TestSaveConcurrentCallsAreRaceSafe verifies that concurrent Save calls do not
// cause data races. It relies on the Go race detector (-race flag).
func (s *postgresRepositorySuite) TestSaveConcurrentCallsAreRaceSafe() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusActive)

	errs := make(chan error, concurrentSavers)

	for i := range concurrentSavers {
		go func(n int) {
			bid := auction.Bid{
				ID:        uuid.NewString(),
				AuctionID: auctionID,
				UserID:    fmt.Sprintf("user-%d", n),
				Amount:    uint64(concurrentBase + n), //nolint:gosec // n is bounded by concurrentSavers
				PlacedAt:  time.Now().UTC(),
			}
			errs <- s.repo.Save(context.Background(), bid)
		}(i)
	}

	for range concurrentSavers {
		s.Require().NoError(<-errs)
	}
}

// TestListActiveStatesReturnsEmptyWhenNoAuctions verifies that a non-nil empty
// slice is returned when the auction table is empty.
func (s *postgresRepositorySuite) TestListActiveStatesReturnsEmptyWhenNoAuctions() {
	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.NotNil(states)
	s.Empty(states)
}

// TestListActiveStatesReturnsActiveAuctionWithNoBids verifies that an active
// auction with no bids is returned with zero CurrentBid and empty BidderID.
func (s *postgresRepositorySuite) TestListActiveStatesReturnsActiveAuctionWithNoBids() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusActive)

	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, 1)

	st := states[0]
	s.Equal(auctionID, st.AuctionID)
	s.Equal(auction.StatusActive, st.Status)
	s.Equal(testStartingPrice, st.StartingPrice)
	s.Equal(testBidAmountZero, st.CurrentBid)
	s.Empty(st.BidderID)
}

// TestListActiveStatesReturnsPendingAuction verifies that pending auctions are
// included in the results (they are not yet finished).
func (s *postgresRepositorySuite) TestListActiveStatesReturnsPendingAuction() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusPending)

	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, 1)
	s.Equal(auction.StatusPending, states[0].Status)
}

// TestListActiveStatesReturnsAuctionSchedule verifies that starts_at and ends_at
// are mapped into auction.State for non-finished auctions.
func (s *postgresRepositorySuite) TestListActiveStatesReturnsAuctionSchedule() {
	auctionID := uuid.NewString()
	startsAt, err := time.Parse(time.RFC3339Nano, "2026-03-18T10:00:00Z")
	s.Require().NoError(err)
	endsAt, err := time.Parse(time.RFC3339Nano, "2026-03-18T11:00:00Z")
	s.Require().NoError(err)

	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO auction (id, title, status, starting_price, starts_at, ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		auctionID, "Scheduled Auction", string(auction.StatusPending), int64(testStartingPrice), startsAt, endsAt,
	)
	s.Require().NoError(err)

	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, 1)

	st := states[0]
	s.True(st.StartsAt.Equal(startsAt))
	s.True(st.EndsAt.Equal(endsAt))
}

// TestListActiveStatesExcludesFinishedAuction verifies that finished auctions
// are not included in the results.
func (s *postgresRepositorySuite) TestListActiveStatesExcludesFinishedAuction() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusFinished)

	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.Empty(states)
}

// TestListActiveStatesReturnsHighestBid verifies that when multiple bids exist
// for a single auction, only the highest one is reflected in the returned State.
func (s *postgresRepositorySuite) TestListActiveStatesReturnsHighestBid() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusActive)

	bids := []auction.Bid{
		{ID: uuid.NewString(), AuctionID: auctionID, UserID: userID, Amount: testBidAmountLow, PlacedAt: time.Now().UTC()},
		{ID: uuid.NewString(), AuctionID: auctionID, UserID: UserID2, Amount: testBidAmountHigh, PlacedAt: time.Now().UTC()},
		{ID: uuid.NewString(), AuctionID: auctionID, UserID: "user-3", Amount: testBidAmountMid, PlacedAt: time.Now().UTC()},
	}

	for _, b := range bids {
		s.Require().NoError(s.repo.Save(context.Background(), b))
	}

	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, 1)

	st := states[0]
	s.Equal(testBidAmountHigh, st.CurrentBid)
	s.Equal(UserID2, st.BidderID)
}

// TestListActiveStatesReturnsStatePerAuction verifies that when multiple
// non-finished auctions exist, each one has its own State entry in the result.
func (s *postgresRepositorySuite) TestListActiveStatesReturnsStatePerAuction() {
	auctionIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}

	for _, id := range auctionIDs {
		s.insertAuction(id, auction.StatusActive)
	}

	s.Require().NoError(s.repo.Save(context.Background(), auction.Bid{
		ID:        uuid.NewString(),
		AuctionID: auctionIDs[0],
		UserID:    userID,
		Amount:    testBidAmountSolo,
		PlacedAt:  time.Now().UTC(),
	}))

	states, err := s.repo.ListActiveStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, len(auctionIDs))

	byID := make(map[string]auction.State, len(states))
	for _, st := range states {
		byID[st.AuctionID] = st
	}

	s.Equal(testBidAmountSolo, byID[auctionIDs[0]].CurrentBid)
	s.Equal(testBidAmountZero, byID[auctionIDs[1]].CurrentBid)
	s.Equal(testBidAmountZero, byID[auctionIDs[2]].CurrentBid)
}

// TestListFinishedStatesReturnsOnlyFinished verifies that finished auctions are
// returned while active and pending auctions are excluded.
func (s *postgresRepositorySuite) TestListFinishedStatesReturnsOnlyFinished() {
	finishedID := uuid.NewString()
	activeID := uuid.NewString()
	pendingID := uuid.NewString()

	s.insertAuction(finishedID, auction.StatusFinished)
	s.insertAuction(activeID, auction.StatusActive)
	s.insertAuction(pendingID, auction.StatusPending)

	states, err := s.repo.ListFinishedStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, 1)
	s.Equal(finishedID, states[0].AuctionID)
	s.Equal(auction.StatusFinished, states[0].Status)
}

// TestListFinishedStatesReturnsHighestBid verifies that finished auction state
// includes the highest recorded bid when bids exist.
func (s *postgresRepositorySuite) TestListFinishedStatesReturnsHighestBid() {
	auctionID := uuid.NewString()
	s.insertAuction(auctionID, auction.StatusFinished)

	bids := []auction.Bid{
		{ID: uuid.NewString(), AuctionID: auctionID, UserID: userID, Amount: testBidAmountLow, PlacedAt: time.Now().UTC()},
		{ID: uuid.NewString(), AuctionID: auctionID, UserID: UserID2, Amount: testBidAmountHigh, PlacedAt: time.Now().UTC()},
		{ID: uuid.NewString(), AuctionID: auctionID, UserID: "user-3", Amount: testBidAmountMid, PlacedAt: time.Now().UTC()},
	}

	for _, b := range bids {
		s.Require().NoError(s.repo.Save(context.Background(), b))
	}

	states, err := s.repo.ListFinishedStates(context.Background())
	s.Require().NoError(err)
	s.Require().Len(states, 1)

	st := states[0]
	s.Equal(testBidAmountHigh, st.CurrentBid)
	s.Equal(UserID2, st.BidderID)
}

// TestFinishExpiredAuctionsMarksOnlyExpired verifies that only auctions with
// ends_at <= now are transitioned to finished.
func (s *postgresRepositorySuite) TestFinishExpiredAuctionsMarksOnlyExpired() {
	now := time.Now().UTC().Truncate(time.Second)
	expiredID := uuid.NewString()
	activeFutureID := uuid.NewString()
	alreadyFinishedID := uuid.NewString()

	s.insertAuctionWithWindow(expiredID, auction.StatusActive, now.Add(-2*time.Hour), now.Add(-time.Minute))
	s.insertAuctionWithWindow(activeFutureID, auction.StatusActive, now.Add(-time.Minute), now.Add(time.Hour))
	s.insertAuctionWithWindow(alreadyFinishedID, auction.StatusFinished, now.Add(-2*time.Hour), now.Add(-time.Minute))

	err := s.repo.FinishExpiredAuctions(context.Background(), now)
	s.Require().NoError(err)

	s.Equal(auction.StatusFinished, s.fetchAuctionStatus(expiredID))
	s.Equal(auction.StatusActive, s.fetchAuctionStatus(activeFutureID))
	s.Equal(auction.StatusFinished, s.fetchAuctionStatus(alreadyFinishedID))
}

// TestFinishExpiredAuctionsIsIdempotent verifies a second call after cleanup
// performs no additional state transition.
func (s *postgresRepositorySuite) TestFinishExpiredAuctionsIsIdempotent() {
	now := time.Now().UTC().Truncate(time.Second)
	expiredID := uuid.NewString()
	s.insertAuctionWithWindow(expiredID, auction.StatusActive, now.Add(-time.Hour), now.Add(-time.Minute))

	err := s.repo.FinishExpiredAuctions(context.Background(), now)
	s.Require().NoError(err)
	s.Equal(auction.StatusFinished, s.fetchAuctionStatus(expiredID))

	err = s.repo.FinishExpiredAuctions(context.Background(), now)
	s.Require().NoError(err)
	s.Equal(auction.StatusFinished, s.fetchAuctionStatus(expiredID))
}
