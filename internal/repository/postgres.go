// Package repository provides persistence implementations for auction data.
// It includes an in-memory implementation for testing and a PostgreSQL-backed
// implementation for production use.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sonubid/api/internal/auction"
)

const (
	// sqlSaveBid inserts a single bid record into the bid table.
	sqlSaveBid = `
		INSERT INTO bid (id, auction_id, user_id, amount, placed_at)
		VALUES ($1, $2, $3, $4, $5)`

	// sqlListActiveStates retrieves the current state for every non-finished auction.
	// For each auction it returns the highest bid via a LEFT JOIN LATERAL; auctions
	// with no bids receive zero values via COALESCE.
	//
	// Note on uint64 ↔ BIGINT: pgx scans BIGINT into int64. The scan targets use
	// int64 and are converted to uint64 after scanning. Values are assumed never to
	// exceed math.MaxInt64.
	sqlListActiveStates = `
		SELECT
			a.id,
			a.status,
			a.starting_price,
			a.starts_at,
			a.ends_at,
			COALESCE(lb.user_id, '')            AS bidder_id,
			COALESCE(lb.amount, 0)              AS current_bid,
			COALESCE(lb.placed_at, a.starts_at) AS updated_at
		FROM auction a
		LEFT JOIN LATERAL (
			SELECT user_id, amount, placed_at
			FROM bid
			WHERE auction_id = a.id
			ORDER BY amount DESC
			LIMIT 1
		) lb ON true
		WHERE a.status != 'finished'`

	// sqlListFinishedStates retrieves state snapshots for every finished auction.
	// It is used by cleanup workers to evict stale finished entries from memory.
	sqlListFinishedStates = `
		SELECT
			a.id,
			a.status,
			a.starting_price,
			a.starts_at,
			a.ends_at,
			COALESCE(lb.user_id, '')            AS bidder_id,
			COALESCE(lb.amount, 0)              AS current_bid,
			COALESCE(lb.placed_at, a.starts_at) AS updated_at
		FROM auction a
		LEFT JOIN LATERAL (
			SELECT user_id, amount, placed_at
			FROM bid
			WHERE auction_id = a.id
			ORDER BY amount DESC
			LIMIT 1
		) lb ON true
		WHERE a.status = 'finished'`

	// sqlFinishExpiredAuctions transitions non-finished auctions to finished
	// when their scheduled end time has passed.
	sqlFinishExpiredAuctions = `
		UPDATE auction
		SET status = 'finished'
		WHERE status != 'finished'
		  AND ends_at <= $1`
)

// PostgresRepository is a PostgreSQL-backed persistence implementation.
// It uses a pgxpool.Pool for all database operations.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a new PostgresRepository backed by the given pool.
// The caller owns the pool lifecycle; PostgresRepository never closes it.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Save persists bid to the PostgreSQL bid table.
// It returns an error if the insert fails (e.g. duplicate ID, FK violation).
// bid.Amount is cast to int64 because pgx does not accept uint64 for BIGINT;
// values are validated upstream and never exceed math.MaxInt64.
func (r *PostgresRepository) Save(ctx context.Context, bid auction.Bid) error {
	//nolint:gosec
	amount := int64(bid.Amount)

	_, err := r.pool.Exec(ctx, sqlSaveBid,
		bid.ID,
		bid.AuctionID,
		bid.UserID,
		amount,
		bid.PlacedAt,
	)
	if err != nil {
		return fmt.Errorf("repository: save bid: %w", err)
	}

	return nil
}

// ListActiveStates returns the current highest-bid state for every auction whose
// status is not 'finished'. Auctions with no bids are returned with zero
// CurrentBid and empty BidderID. An empty (non-nil) slice is returned when no
// non-finished auctions exist.
// BIGINT columns are scanned into int64 and converted to uint64 after scanning;
// values from this query are always non-negative and never exceed math.MaxInt64.
func (r *PostgresRepository) ListActiveStates(ctx context.Context) ([]auction.State, error) {
	states, err := r.listStatesByQuery(ctx, sqlListActiveStates)
	if err != nil {
		return nil, fmt.Errorf("repository: list active states: %w", err)
	}

	return states, nil
}

// ListFinishedStates returns the current state snapshot for every auction whose
// status is 'finished'.
func (r *PostgresRepository) ListFinishedStates(ctx context.Context) ([]auction.State, error) {
	states, err := r.listStatesByQuery(ctx, sqlListFinishedStates)
	if err != nil {
		return nil, fmt.Errorf("repository: list finished states: %w", err)
	}

	return states, nil
}

// FinishExpiredAuctions marks every non-finished auction with ends_at <= now as
// finished.
func (r *PostgresRepository) FinishExpiredAuctions(ctx context.Context, now time.Time) error {
	_, err := r.pool.Exec(ctx, sqlFinishExpiredAuctions, now)
	if err != nil {
		return fmt.Errorf("repository: finish expired auctions: %w", err)
	}

	return nil
}

func (r *PostgresRepository) listStatesByQuery(ctx context.Context, query string) ([]auction.State, error) {
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query states: %w", err)
	}
	defer rows.Close()

	states := make([]auction.State, 0)

	for rows.Next() {
		var (
			id            string
			status        string
			startingPrice int64
			startsAt      time.Time
			endsAt        time.Time
			bidderID      string
			currentBid    int64
			updatedAt     time.Time
		)

		if err := rows.Scan(&id, &status, &startingPrice, &startsAt, &endsAt, &bidderID, &currentBid, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan state: %w", err)
		}

		//nolint:gosec
		states = append(states, auction.State{
			AuctionID:     id,
			Status:        auction.Status(status),
			StartingPrice: uint64(startingPrice),
			StartsAt:      startsAt,
			EndsAt:        endsAt,
			BidderID:      bidderID,
			CurrentBid:    uint64(currentBid),
			UpdatedAt:     updatedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate states: %w", err)
	}

	return states, nil
}
