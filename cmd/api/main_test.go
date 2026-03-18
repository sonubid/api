package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/store"
)

const (
	testSyncInterval          = 10 * time.Millisecond
	testCleanupInterval       = 15 * time.Millisecond
	testSyncWaitTimeout       = time.Second
	testAuctionIDExisting     = "auction-existing"
	testAuctionIDNew          = "auction-new"
	testAuctionIDInvalid      = ""
	testExistingStartingPrice = uint64(100)
	testExistingCurrentBid    = uint64(500)
	testProviderStartingPrice = uint64(50)
	testProviderCurrentBid    = uint64(60)
	testEnvSyncInterval       = "20ms"
	testEnvCleanupInterval    = "25ms"
	testEnvInvalidDuration    = "abc"
	testEnvZeroDuration       = "0s"
)

type mainSuite struct {
	suite.Suite
}

func TestMainSuite(t *testing.T) {
	suite.Run(t, new(mainSuite))
}

func (s *mainSuite) TestLoadStoreSyncIntervalFromEnvDefault() {
	s.T().Setenv(storeSyncIntervalEnvVar, "")

	interval, err := loadStoreSyncIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().NoError(err)
	s.Equal(storeSyncIntervalDefault, interval)
}

func (s *mainSuite) TestLoadStoreSyncIntervalFromEnvCustomValue() {
	s.T().Setenv(storeSyncIntervalEnvVar, testEnvSyncInterval)

	interval, err := loadStoreSyncIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().NoError(err)
	s.Equal(testSyncInterval*2, interval)
}

func (s *mainSuite) TestLoadStoreSyncIntervalFromEnvInvalidValue() {
	s.T().Setenv(storeSyncIntervalEnvVar, testEnvInvalidDuration)

	_, err := loadStoreSyncIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().Error(err)
	s.ErrorContains(err, "parse duration")
}

func (s *mainSuite) TestLoadStoreSyncIntervalFromEnvRejectsNonPositive() {
	s.T().Setenv(storeSyncIntervalEnvVar, testEnvZeroDuration)

	_, err := loadStoreSyncIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().Error(err)
	s.ErrorContains(err, "greater than zero")
}

func (s *mainSuite) TestLoadAuctionCleanupIntervalFromEnvDefault() {
	s.T().Setenv(auctionCleanupIntervalEnv, "")

	interval, err := loadAuctionCleanupIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().NoError(err)
	s.Equal(storeSyncIntervalDefault, interval)
}

func (s *mainSuite) TestLoadAuctionCleanupIntervalFromEnvCustomValue() {
	s.T().Setenv(auctionCleanupIntervalEnv, testEnvCleanupInterval)

	interval, err := loadAuctionCleanupIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().NoError(err)
	s.Equal(testCleanupInterval+10*time.Millisecond, interval)
}

func (s *mainSuite) TestLoadAuctionCleanupIntervalFromEnvInvalidValue() {
	s.T().Setenv(auctionCleanupIntervalEnv, testEnvInvalidDuration)

	_, err := loadAuctionCleanupIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().Error(err)
	s.ErrorContains(err, "parse duration")
}

func (s *mainSuite) TestLoadAuctionCleanupIntervalFromEnvRejectsNonPositive() {
	s.T().Setenv(auctionCleanupIntervalEnv, testEnvZeroDuration)

	_, err := loadAuctionCleanupIntervalFromEnv(slog.New(slog.DiscardHandler))

	s.Require().Error(err)
	s.ErrorContains(err, "greater than zero")
}

func (s *mainSuite) TestSyncStoreFromDBLoadsOnlyAbsentAuctions() {
	ctx := context.Background()
	st := store.New()
	logger := discardLogger()

	s.Require().NoError(st.LoadState(ctx, auction.State{
		AuctionID:     testAuctionIDExisting,
		Status:        auction.StatusActive,
		StartingPrice: testExistingStartingPrice,
		CurrentBid:    testExistingCurrentBid,
		BidderID:      "keeper",
		StartsAt:      time.Now().Add(-time.Hour),
		EndsAt:        time.Now().Add(time.Hour),
	}))

	provider := &mockActiveStateProvider{
		listActiveStatesFn: func(context.Context) ([]auction.State, error) {
			return []auction.State{
				{
					AuctionID:     testAuctionIDExisting,
					Status:        auction.StatusActive,
					StartingPrice: testProviderStartingPrice,
					CurrentBid:    testProviderCurrentBid,
					BidderID:      "provider",
					StartsAt:      time.Now().Add(-time.Hour),
					EndsAt:        time.Now().Add(time.Hour),
				},
				{
					AuctionID:     testAuctionIDNew,
					Status:        auction.StatusPending,
					StartingPrice: testProviderStartingPrice,
					StartsAt:      time.Now().Add(time.Hour),
					EndsAt:        time.Now().Add(2 * time.Hour),
				},
			}, nil
		},
	}

	err := syncStoreFromDB(ctx, logger, st, provider)

	s.Require().NoError(err)

	existing, err := st.GetState(ctx, testAuctionIDExisting)
	s.Require().NoError(err)
	s.Equal(testExistingCurrentBid, existing.CurrentBid)
	s.Equal("keeper", existing.BidderID)

	newState, err := st.GetState(ctx, testAuctionIDNew)
	s.Require().NoError(err)
	s.Equal(testProviderStartingPrice, newState.StartingPrice)
	s.Equal(auction.StatusPending, newState.Status)
}

func (s *mainSuite) TestSyncStoreFromDBReturnsProviderError() {
	ctx := context.Background()
	st := store.New()
	logger := discardLogger()

	provider := &mockActiveStateProvider{
		listActiveStatesFn: func(context.Context) ([]auction.State, error) {
			return nil, errors.New("db unavailable")
		},
	}

	err := syncStoreFromDB(ctx, logger, st, provider)

	s.Require().Error(err)
	s.ErrorContains(err, "list active states")
}

func (s *mainSuite) TestSyncStoreFromDBReturnsStoreError() {
	ctx := context.Background()
	st := store.New()
	logger := discardLogger()

	provider := &mockActiveStateProvider{
		listActiveStatesFn: func(context.Context) ([]auction.State, error) {
			return []auction.State{
				{AuctionID: testAuctionIDInvalid},
			}, nil
		},
	}

	err := syncStoreFromDB(ctx, logger, st, provider)

	s.Require().Error(err)
	s.ErrorContains(err, "load state if absent")
}

func (s *mainSuite) TestStartStoreSyncLoadsNewAuctionAndStopsOnCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := store.New()
	provider := &mockActiveStateProvider{
		listActiveStatesFn: func(context.Context) ([]auction.State, error) {
			return []auction.State{
				{
					AuctionID:     testAuctionIDNew,
					Status:        auction.StatusPending,
					StartingPrice: testProviderStartingPrice,
					StartsAt:      time.Now().Add(time.Hour),
					EndsAt:        time.Now().Add(2 * time.Hour),
				},
			}, nil
		},
	}

	wg := &sync.WaitGroup{}
	startStoreSync(ctx, discardLogger(), wg, testSyncInterval, st, provider)

	s.Require().Eventually(func() bool {
		_, err := st.GetState(context.Background(), testAuctionIDNew)
		return err == nil
	}, testSyncWaitTimeout, testSyncInterval)

	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(testSyncWaitTimeout):
		s.Fail("store sync goroutine did not stop after context cancellation")
	}
}

func (s *mainSuite) TestCleanupFinishedAuctionsMarksAndEvictsAuctions() {
	ctx := context.Background()
	logger := discardLogger()
	evicter := &mockStateEvicter{}
	finalizer := &mockAuctionFinalizer{
		finishExpiredAuctionsFn: func(context.Context, time.Time) ([]string, error) {
			return []string{testAuctionIDExisting, testAuctionIDNew}, nil
		},
	}

	err := cleanupFinishedAuctions(ctx, logger, evicter, finalizer, time.Now().UTC())

	s.Require().NoError(err)
	s.Equal([]string{testAuctionIDExisting, testAuctionIDNew}, evicter.DeletedIDs())
}

func (s *mainSuite) TestCleanupFinishedAuctionsReturnsFinalizerError() {
	ctx := context.Background()
	logger := discardLogger()
	evicter := &mockStateEvicter{}
	finalizer := &mockAuctionFinalizer{
		finishExpiredAuctionsFn: func(context.Context, time.Time) ([]string, error) {
			return nil, errors.New("db failure")
		},
	}

	err := cleanupFinishedAuctions(ctx, logger, evicter, finalizer, time.Now().UTC())

	s.Require().Error(err)
	s.ErrorContains(err, "finish expired auctions")
	s.Empty(evicter.DeletedIDs())
}

func (s *mainSuite) TestCleanupFinishedAuctionsReturnsEvicterError() {
	ctx := context.Background()
	logger := discardLogger()
	evicter := &mockStateEvicter{
		deleteStateFn: func(context.Context, string) error {
			return errors.New("cache failure")
		},
	}
	finalizer := &mockAuctionFinalizer{
		finishExpiredAuctionsFn: func(context.Context, time.Time) ([]string, error) {
			return []string{testAuctionIDExisting}, nil
		},
	}

	err := cleanupFinishedAuctions(ctx, logger, evicter, finalizer, time.Now().UTC())

	s.Require().Error(err)
	s.ErrorContains(err, "evict auction")
}

func (s *mainSuite) TestStartAuctionCleanupRunsAndStopsOnCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evicter := &mockStateEvicter{}
	finalizer := &mockAuctionFinalizer{
		finishExpiredAuctionsFn: func(context.Context, time.Time) ([]string, error) {
			return []string{testAuctionIDNew}, nil
		},
	}

	wg := &sync.WaitGroup{}
	startAuctionCleanup(ctx, discardLogger(), wg, testSyncInterval, evicter, finalizer)

	s.Require().Eventually(func() bool {
		return len(evicter.DeletedIDs()) > 0
	}, testSyncWaitTimeout, testSyncInterval)

	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(testSyncWaitTimeout):
		s.Fail("auction cleanup goroutine did not stop after context cancellation")
	}
}

type mockActiveStateProvider struct {
	mu                 sync.Mutex
	listActiveStatesFn func(ctx context.Context) ([]auction.State, error)
	calls              int
}

func (m *mockActiveStateProvider) ListActiveStates(ctx context.Context) ([]auction.State, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.listActiveStatesFn != nil {
		return m.listActiveStatesFn(ctx)
	}

	return nil, errors.New("mockActiveStateProvider: listActiveStatesFn is nil")
}

type mockAuctionFinalizer struct {
	mu                      sync.Mutex
	finishExpiredAuctionsFn func(ctx context.Context, now time.Time) ([]string, error)
	calls                   int
}

func (m *mockAuctionFinalizer) FinishExpiredAuctions(ctx context.Context, now time.Time) ([]string, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.finishExpiredAuctionsFn != nil {
		return m.finishExpiredAuctionsFn(ctx, now)
	}

	return nil, errors.New("mockAuctionFinalizer: finishExpiredAuctionsFn is nil")
}

type mockStateEvicter struct {
	mu            sync.Mutex
	deleteStateFn func(ctx context.Context, auctionID string) error
	deletedIDs    []string
}

func (m *mockStateEvicter) DeleteState(ctx context.Context, auctionID string) error {
	m.mu.Lock()
	m.deletedIDs = append(m.deletedIDs, auctionID)
	m.mu.Unlock()

	if m.deleteStateFn != nil {
		return m.deleteStateFn(ctx, auctionID)
	}

	return nil
}

func (m *mockStateEvicter) DeletedIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]string, len(m.deletedIDs))
	copy(out, m.deletedIDs)

	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
