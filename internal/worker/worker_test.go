package worker_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/worker"
)

type workerSuite struct {
	suite.Suite

	ctx    context.Context
	cancel context.CancelFunc
	repo   *mockRepository
	queue  *mockQueue
	w      *worker.Worker
}

func TestWorkerSuite(t *testing.T) { suite.Run(t, new(workerSuite)) }

func (s *workerSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.repo = &mockRepository{}
	s.queue = newMockQueue()
	s.w = worker.New(s.repo, s.queue, discardLogger())
}

func (s *workerSuite) TearDownTest() {
	s.cancel()
}

func (s *workerSuite) TestStartSavesEnqueuedEvent() {
	event := makeBidEvent(auctionOne, userOne, bidAmount)
	s.queue.send(event)
	s.queue.Close()

	s.w.Start(s.ctx, workerID)

	require.Equal(s.T(), 1, s.repo.saveCount())
	require.Equal(s.T(), event.Bid, s.repo.firstSaveCall())
}

func (s *workerSuite) TestStartSavesMultipleEvents() {
	for range eventCount {
		s.queue.send(makeBidEvent(auctionOne, userOne, bidAmount))
	}
	s.queue.Close()

	s.w.Start(s.ctx, workerID)

	require.Equal(s.T(), eventCount, s.repo.saveCount())
}

func (s *workerSuite) TestStartExitsWhenQueueIsClosed() {
	s.queue.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.w.Start(s.ctx, workerID)
	}()

	select {
	case <-done:
	case <-time.After(waitTimeout):
		s.Fail("Start did not exit after queue was closed")
	}
}

func (s *workerSuite) TestStartExitsWhenContextIsCancelled() {
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		s.w.Start(s.ctx, workerID)
	}()

	<-started
	s.cancel()

	select {
	case <-done:
	case <-time.After(waitTimeout):
		s.Fail("Start did not exit after context cancellation")
	}
}

func (s *workerSuite) TestStartContinuesOnRepositoryFailure() {
	saveErr := errors.New("db unavailable")
	s.repo.saveFn = func(_ context.Context, _ auction.Bid) error { return saveErr }

	event := makeBidEvent(auctionOne, userOne, bidAmount)
	s.queue.send(event)
	s.queue.Close()

	s.w.Start(s.ctx, workerID)

	require.Equal(s.T(), 1, s.repo.saveCount())
}

func (s *workerSuite) TestStartContinuesAfterRepositoryFailure() {
	var callCount atomic.Int32
	s.repo.saveFn = func(_ context.Context, _ auction.Bid) error {
		if callCount.Add(1) == 1 {
			return errors.New("transient error")
		}
		return nil
	}

	s.queue.send(makeBidEvent(auctionOne, userOne, bidAmount))
	s.queue.send(makeBidEvent(auctionOne, userTwo, bidAmount+step))
	s.queue.Close()

	s.w.Start(s.ctx, workerID)

	require.Equal(s.T(), 2, s.repo.saveCount())
}

func (s *workerSuite) TestNewWithNilLoggerDoesNotPanic() {
	w := worker.New(s.repo, s.queue, nil)
	s.queue.Close()

	require.NotPanics(s.T(), func() { w.Start(s.ctx, workerID) })
}

func (s *workerSuite) TestStartHandlesConcurrentWorkers() {
	for range eventCount {
		s.queue.send(makeBidEvent(auctionOne, userOne, bidAmount))
	}
	s.queue.Close()

	var wg sync.WaitGroup

	for i := range workerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s.w.Start(s.ctx, id)
		}(i)
	}

	wg.Wait()

	require.Equal(s.T(), eventCount, s.repo.saveCount())
}
