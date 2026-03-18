package repository_test

import (
	"context"
	"time"

	"github.com/sonubid/api/internal/auction"
)

// insertAuction inserts a minimal auction row into the database for use by
// integration test methods within the suite.
func (s *postgresRepositorySuite) insertAuction(id string, status auction.Status) {
	s.T().Helper()

	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO auction (id, title, status, starting_price, starts_at, ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, "Test Auction", string(status), int64(testStartingPrice), now, now.Add(time.Hour),
	)
	s.Require().NoError(err)
}

// insertAuctionWithWindow inserts an auction row with explicit starts_at and
// ends_at values.
func (s *postgresRepositorySuite) insertAuctionWithWindow(id string, status auction.Status, startsAt, endsAt time.Time) {
	s.T().Helper()

	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO auction (id, title, status, starting_price, starts_at, ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, "Test Auction", string(status), int64(testStartingPrice), startsAt, endsAt,
	)
	s.Require().NoError(err)
}

// fetchAuctionStatus returns the persisted status for a single auction.
func (s *postgresRepositorySuite) fetchAuctionStatus(id string) auction.Status {
	s.T().Helper()

	var status string
	err := s.pool.QueryRow(context.Background(), `SELECT status FROM auction WHERE id = $1`, id).Scan(&status)
	s.Require().NoError(err)

	return auction.Status(status)
}
