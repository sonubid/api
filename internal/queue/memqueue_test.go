package queue_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/queue"
)

const (
	auctionID     = "auction-1"
	userID        = "user-1"
	bidAmount     = 500
	queueSize     = 100
	concWorkers   = 10
	bidsPerWorker = 5
	waitTimeout   = time.Second
)

type memQueueSuite struct {
	suite.Suite

	q *queue.MemQueue
}

func TestMemQueueSuite(t *testing.T) {
	suite.Run(t, new(memQueueSuite))
}

func (s *memQueueSuite) SetupTest() {
	s.q = queue.New()
}

// TestEnqueueSucceeds verifies that a single event can be enqueued without error.
func (s *memQueueSuite) TestEnqueueSucceeds() {
	err := s.q.Enqueue(makeBidEvent(bidAmount))
	s.Require().NoError(err)
}

// TestEventsReceivesEnqueuedEvent verifies that an enqueued event is emitted
// by the Events channel.
func (s *memQueueSuite) TestEventsReceivesEnqueuedEvent() {
	event := makeBidEvent(bidAmount)
	s.Require().NoError(s.q.Enqueue(event))

	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()

	select {
	case got := <-s.q.Events():
		s.Equal(event, got)
	case <-timer.C:
		s.Fail("timed out waiting for event")
	}
}

// TestEventsPreservesOrder verifies that multiple events are emitted in FIFO order.
func (s *memQueueSuite) TestEventsPreservesOrder() {
	amounts := []uint64{100, 200, 300}
	for _, a := range amounts {
		s.Require().NoError(s.q.Enqueue(makeBidEvent(a)))
	}

	for _, want := range amounts {
		timer := time.NewTimer(waitTimeout)
		defer timer.Stop()

		select {
		case got := <-s.q.Events():
			s.Equal(want, got.Bid.Amount)
		case <-timer.C:
			s.Fail("timed out waiting for event")
		}
	}
}

// TestEnqueueReturnsErrQueueFullWhenBufferFull verifies that Enqueue returns
// ErrQueueFull immediately when the buffer is at capacity, without blocking.
func (s *memQueueSuite) TestEnqueueReturnsErrQueueFullWhenBufferFull() {
	for i := range queueSize {
		err := s.q.Enqueue(makeBidEvent(uint64(i + 1)))
		s.Require().NoError(err, "unexpected error on enqueue %d", i)
	}

	err := s.q.Enqueue(makeBidEvent(bidAmount))
	s.Require().ErrorIs(err, queue.ErrQueueFull)
}

// TestEnqueueAfterDrainSucceeds verifies that Enqueue succeeds again after
// events are consumed from a previously full queue.
func (s *memQueueSuite) TestEnqueueAfterDrainSucceeds() {
	for i := range queueSize {
		s.Require().NoError(s.q.Enqueue(makeBidEvent(uint64(i + 1))))
	}
	s.Require().ErrorIs(s.q.Enqueue(makeBidEvent(bidAmount)), queue.ErrQueueFull)

	<-s.q.Events()

	s.Require().NoError(s.q.Enqueue(makeBidEvent(bidAmount)))
}

// TestConcurrentEnqueueIsSafe verifies that concurrent calls to Enqueue do not
// cause data races and that each successful enqueue produces a consumable event.
func (s *memQueueSuite) TestConcurrentEnqueueIsSafe() {
	results := make(chan error, concWorkers*bidsPerWorker)

	var wg sync.WaitGroup
	for range concWorkers {
		wg.Go(func() {
			for range bidsPerWorker {
				results <- s.q.Enqueue(makeBidEvent(bidAmount))
			}
		})
	}
	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			s.ErrorIs(err, queue.ErrQueueFull)
		}
	}

	consumed := 0
	for range successCount {
		timer := time.NewTimer(waitTimeout)
		defer timer.Stop()

		select {
		case <-s.q.Events():
			consumed++
		case <-timer.C:
			s.Fail("timed out draining events")
		}
	}
	s.Equal(successCount, consumed)
}

// TestCloseSignalsConsumer verifies that Close shuts down the Events channel so
// that a ranging consumer can detect the shutdown and exit.
func (s *memQueueSuite) TestCloseSignalsConsumer() {
	s.q.Close()

	_, ok := <-s.q.Events()
	s.False(ok, "Events channel should be closed after Close")
}

// TestCloseIsIdempotent verifies that calling Close multiple times does not panic.
func (s *memQueueSuite) TestCloseIsIdempotent() {
	s.NotPanics(func() {
		s.q.Close()
		s.q.Close()
	})
}
