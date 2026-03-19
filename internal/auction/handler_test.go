package auction_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/hub"
)

// TestNewHandlerReturnsNonNilHandler verifies that NewHandler returns a usable route handler.
func (s *handlerSuite) TestNewHandlerReturnsNonNilHandler() {
	proc := &mockProcessor{}
	h := hub.NewHub()

	got := auction.NewHandler(proc, "", h, discardLogger())

	s.Require().NotNil(got)
}

// TestRegisterRoutesRegistersAuctionWSRoute verifies that RegisterRoutes mounts
// the auction WebSocket route. A plain HTTP request is expected to fail the
// WebSocket upgrade (not 404).
func (s *handlerSuite) TestRegisterRoutesRegistersAuctionWSRoute() {
	proc := &mockProcessor{}
	srv, _ := s.newServerWithProc(proc)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		srv.URL+"/api/v1/ws/auction/test-auction", nil)
	s.Require().NoError(err)

	client := &http.Client{}
	resp, err := client.Do(req) //nolint:gosec // URL is constructed from httptest.Server in tests
	s.Require().NoError(err)

	defer resp.Body.Close()

	s.Require().NotEqual(http.StatusNotFound, resp.StatusCode,
		"auction WS route must be registered")
}

// TestMsgHandlerCallsProcessBidOnValidMessage verifies that a well-formed bid
// JSON message triggers a ProcessBid call with the correct fields.
func (s *handlerSuite) TestMsgHandlerCallsProcessBidOnValidMessage() {
	proc := &mockProcessor{}
	srv, _ := s.newServerWithProc(proc)

	conn := dialWS(s.T(), srv, auctionOne)
	sendMsg(s.T(), conn, validBidJSON)

	s.Require().Eventually(
		func() bool { return proc.callCount() == 1 },
		waitTimeout,
		pollInterval,
		"ProcessBid must be called once",
	)

	call := proc.firstCall()
	s.Require().Equal(auctionOne, call.bid.AuctionID)
	s.Require().Equal(userOne, call.bid.UserID)
	s.Require().Equal(bidAmount, call.bid.Amount)
	s.Require().NotEmpty(call.bid.ID)
	s.Require().False(call.bid.PlacedAt.IsZero())
}

// TestMsgHandlerDropsInvalidJSON verifies that a malformed message does not
// trigger ProcessBid. A valid message is sent immediately after to confirm the
// handler is still running.
func (s *handlerSuite) TestMsgHandlerDropsInvalidJSON() {
	proc := &mockProcessor{}
	srv, _ := s.newServerWithProc(proc)

	conn := dialWS(s.T(), srv, auctionOne)
	sendMsg(s.T(), conn, invalidBidJSON)
	sendMsg(s.T(), conn, validBidJSON)

	s.Require().Eventually(
		func() bool { return proc.callCount() == 1 },
		waitTimeout,
		pollInterval,
		"only the valid message must reach ProcessBid",
	)
}

// TestMsgHandlerContinuesAfterRejectedBid verifies that a ProcessBid error
// does not close the connection — subsequent messages are still processed.
func (s *handlerSuite) TestMsgHandlerContinuesAfterRejectedBid() {
	proc := &mockProcessor{}
	proc.processFn = func(_ context.Context, _ auction.Bid, _ []byte) error {
		return errors.New("bid rejected")
	}

	srv, _ := s.newServerWithProc(proc)

	conn := dialWS(s.T(), srv, auctionOne)
	sendMsg(s.T(), conn, validBidJSON)
	sendMsg(s.T(), conn, validBidJSON)

	s.Require().Eventually(
		func() bool { return proc.callCount() == 2 },
		waitTimeout,
		pollInterval,
		"both messages must be processed despite the first rejection",
	)
}

// TestNewHandlerLogsWarningOnEmptyAllowedOrigin verifies that NewHandler with
// an empty AllowedOrigin still returns a functioning handler.
func (s *handlerSuite) TestNewHandlerLogsWarningOnEmptyAllowedOrigin() {
	proc := &mockProcessor{}
	h := hub.NewHub()
	got := auction.NewHandler(proc, "", h, discardLogger())

	mux := http.NewServeMux()
	got.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	s.T().Cleanup(srv.Close)

	conn := dialWS(s.T(), srv, auctionOne)
	sendMsg(s.T(), conn, validBidJSON)

	s.Require().Eventually(
		func() bool { return proc.callCount() == 1 },
		waitTimeout,
		pollInterval,
	)
}

// newServerWithProc is a suite-local helper that builds a test server and
// returns the server and hub.
func (s *handlerSuite) newServerWithProc(proc auction.BidProcessor) (*httptest.Server, *hub.Hub) {
	s.T().Helper()

	srv, h := newTestServer(s.T(), proc)

	return srv, h
}
