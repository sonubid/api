package hub_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/hub"
)

const (
	handlerAuction = "auction-1"
	incomingBid    = `{"amount":500}`
	broadcastBid   = `{"bid":750}`
	multiBid       = `{"bid":1000}`
)

type handlerSuite struct {
	suite.Suite
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(handlerSuite))
}

// ---------------------------------------------------------------------------
// Handler — upgrade and registration
// ---------------------------------------------------------------------------

func (s *handlerSuite) TestHandlerRegistersClientOnConnect() {
	h := newHub()
	srv, conn, _ := dialHandler(s.T(), h, handlerAuction, nil)
	defer srv.Close()
	defer func() { _ = conn.CloseNow() }()

	waitForCount(s.T(), h, handlerAuction, 1)
	s.Equal(1, h.ClientCount(handlerAuction))
}

func (s *handlerSuite) TestHandlerUnregistersClientOnDisconnect() {
	h := newHub()
	srv, conn, _ := dialHandler(s.T(), h, handlerAuction, nil)
	defer srv.Close()

	waitForCount(s.T(), h, handlerAuction, 1)

	conn.Close(websocket.StatusNormalClosure, "bye")
	waitForCount(s.T(), h, handlerAuction, 0)

	s.Equal(0, h.ClientCount(handlerAuction))
}

// ---------------------------------------------------------------------------
// Handler — message routing
// ---------------------------------------------------------------------------

func (s *handlerSuite) TestHandlerInvokesHandlerWithIncomingMessage() {
	recv := make(chan []byte, 1)
	h := newHub()
	srv, conn, _ := dialHandler(s.T(), h, handlerAuction, func(msg []byte) {
		recv <- msg
	})
	defer srv.Close()
	defer func() { _ = conn.CloseNow() }()

	waitForCount(s.T(), h, handlerAuction, 1)

	err := conn.Write(context.Background(), websocket.MessageText, []byte(incomingBid))
	s.Require().NoError(err)

	s.Equal(incomingBid, string(waitMsg(s.T(), recv)))
}

func (s *handlerSuite) TestHandlerBroadcastReachesConnectedClient() {
	h := newHub()
	recv := make(chan []byte, 1)

	_, conn := newTestPair(s.T(), h, handlerAuction, recv)
	defer func() { _ = conn.CloseNow() }()

	h.Broadcast(handlerAuction, []byte(broadcastBid))

	s.Equal(broadcastBid, string(waitMsg(s.T(), recv)))
}

func (s *handlerSuite) TestHandlerMultipleConnectionsSameAuction() {
	h := newHub()
	recv1 := make(chan []byte, 1)
	recv2 := make(chan []byte, 1)

	srv1, c1 := newTestPair(s.T(), h, handlerAuction, recv1)
	defer srv1.Close()
	defer func() { _ = c1.CloseNow() }()

	srv2, c2 := newTestPair(s.T(), h, handlerAuction, recv2)
	defer srv2.Close()
	defer func() { _ = c2.CloseNow() }()

	h.Broadcast(handlerAuction, []byte(multiBid))

	s.Equal(multiBid, string(waitMsg(s.T(), recv1)))
	s.Equal(multiBid, string(waitMsg(s.T(), recv2)))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// dialHandler starts an httptest.Server backed by hub.Handler with the given
// msgHandler, dials it and returns (server, clientConn, msgHandler channel).
// Unlike newTestPair, it does not start a receive goroutine — callers that
// need to read broadcasts should use newTestPair instead.
func dialHandler(
	t *testing.T,
	h *hub.Hub,
	auctionID string,
	msgHandler func([]byte),
) (*httptest.Server, *websocket.Conn, func([]byte)) {
	t.Helper()

	if msgHandler == nil {
		msgHandler = func(_ []byte) {}
	}

	srv := httptest.NewServer(hub.Handler(h, auctionID, msgHandler, testAcceptOpts()))
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):]
	conn, resp, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err, "dial websocket")
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	return srv, conn, msgHandler
}
