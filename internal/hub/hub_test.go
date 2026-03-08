package hub_test

import (
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/hub"
)

const (
	auctionOne   = "auction-1"
	auctionTwo   = "auction-2"
	ghostAuction = "ghost-auction"

	// sendBufSize mirrors the internal sendBufferSize constant from the hub
	// package, used to validate the broadcast drop behaviour.
	sendBufSize = 256
)

type hubSuite struct {
	suite.Suite
}

func TestHubSuite(t *testing.T) {
	suite.Run(t, new(hubSuite))
}

// ---------------------------------------------------------------------------
// NewHub
// ---------------------------------------------------------------------------

func (s *hubSuite) TestNewHubIsEmpty() {
	h := newHub()
	s.Equal(0, h.ClientCount("any-auction"))
}

// ---------------------------------------------------------------------------
// Register / ClientCount
// ---------------------------------------------------------------------------

func (s *hubSuite) TestRegisterAddsClientToRoom() {
	h := newHub()

	srv, conn := newTestPair(s.T(), h, auctionOne, nil)
	defer srv.Close()
	defer func() { _ = conn.CloseNow() }()

	s.Equal(1, h.ClientCount(auctionOne))
}

func (s *hubSuite) TestRegisterMultipleClientsInSameRoom() {
	h := newHub()

	srv1, c1 := newTestPair(s.T(), h, auctionOne, nil)
	defer srv1.Close()
	defer func() { _ = c1.CloseNow() }()

	srv2, c2 := newTestPair(s.T(), h, auctionOne, nil)
	defer srv2.Close()
	defer func() { _ = c2.CloseNow() }()

	s.Equal(2, h.ClientCount(auctionOne))
}

func (s *hubSuite) TestRegisterDifferentRoomsAreIndependent() {
	h := newHub()

	srv1, c1 := newTestPair(s.T(), h, auctionOne, nil)
	defer srv1.Close()
	defer func() { _ = c1.CloseNow() }()

	srv2, c2 := newTestPair(s.T(), h, auctionTwo, nil)
	defer srv2.Close()
	defer func() { _ = c2.CloseNow() }()

	s.Equal(1, h.ClientCount(auctionOne))
	s.Equal(1, h.ClientCount(auctionTwo))
}

// ---------------------------------------------------------------------------
// Unregister
// ---------------------------------------------------------------------------

func (s *hubSuite) TestUnregisterRemovesClientWhenConnectionCloses() {
	h := newHub()

	srv, conn := newTestPair(s.T(), h, auctionOne, nil)
	defer srv.Close()

	s.Equal(1, h.ClientCount(auctionOne))

	// Closing the dialer triggers ReadPump to exit, which causes the handler
	// to call h.Unregister.
	conn.Close(websocket.StatusNormalClosure, "bye")
	waitForCount(s.T(), h, auctionOne, 0)

	s.Equal(0, h.ClientCount(auctionOne))
}

func (s *hubSuite) TestUnregisterEmptyRoomIsDeletedFromMap() {
	h := newHub()

	srv, conn := newTestPair(s.T(), h, auctionOne, nil)
	defer srv.Close()

	conn.Close(websocket.StatusNormalClosure, "bye")
	waitForCount(s.T(), h, auctionOne, 0)

	// A missing room reports 0 — same observable behaviour.
	s.Equal(0, h.ClientCount(auctionOne))
}

func (s *hubSuite) TestUnregisterNonExistentAuctionIsNoop() {
	h := newHub()
	ghost := hub.NewClient(nil)
	s.NotPanics(func() {
		h.Unregister(ghostAuction, ghost)
	})
}

// ---------------------------------------------------------------------------
// Broadcast
// ---------------------------------------------------------------------------

func (s *hubSuite) TestBroadcastMessageReachesAllClientsInRoom() {
	h := newHub()
	recv1 := make(chan []byte, 1)
	recv2 := make(chan []byte, 1)

	const bidMsg = `{"bid":100}`

	srv1, c1 := newTestPair(s.T(), h, auctionOne, recv1)
	defer srv1.Close()
	defer func() { _ = c1.CloseNow() }()

	srv2, c2 := newTestPair(s.T(), h, auctionOne, recv2)
	defer srv2.Close()
	defer func() { _ = c2.CloseNow() }()

	h.Broadcast(auctionOne, []byte(bidMsg))

	s.Equal(bidMsg, string(waitMsg(s.T(), recv1)))
	s.Equal(bidMsg, string(waitMsg(s.T(), recv2)))
}

func (s *hubSuite) TestBroadcastDoesNotReachOtherRoom() {
	h := newHub()
	recv := make(chan []byte, 1)

	srv1, c1 := newTestPair(s.T(), h, auctionOne, nil)
	defer srv1.Close()
	defer func() { _ = c1.CloseNow() }()

	srv2, c2 := newTestPair(s.T(), h, auctionTwo, recv)
	defer srv2.Close()
	defer func() { _ = c2.CloseNow() }()

	h.Broadcast(auctionOne, []byte(`{"bid":999}`))

	s.Nil(drainNonBlocking(recv))
}

func (s *hubSuite) TestBroadcastEmptyRoomIsNoop() {
	h := newHub()
	s.NotPanics(func() {
		h.Broadcast(ghostAuction, []byte(`{"bid":1}`))
	})
}

func (s *hubSuite) TestBroadcastDropsMessageWhenSendChannelFull() {
	h := newHub()

	// Register a raw client directly (no WritePump started) so its send
	// channel fills up without draining.
	srv, conn := newTestPair(s.T(), h, auctionOne, nil)
	defer srv.Close()
	defer func() { _ = conn.CloseNow() }()

	// Fill the send channel beyond its capacity; extra messages must be
	// silently dropped without blocking or panicking.
	total := sendBufSize + 10
	s.NotPanics(func() {
		for i := range total {
			h.Broadcast(auctionOne, []byte(bidPayload(i)))
		}
	})
}
