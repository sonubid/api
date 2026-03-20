// Package auction_test provides black-box tests for the auction package.
package auction_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/hub"
)

const (
	auctionOne     = "auction-1"
	userOne        = "user-1"
	validBidJSON   = `{"userId":"user-1","amount":500}`
	invalidBidJSON = `not-valid-json`
	bidAmount      = uint64(500)
	waitTimeout    = 2 * time.Second
	pollInterval   = 10 * time.Millisecond
)

// processorCall records a single call to ProcessBid.
type processorCall struct {
	bid auction.Bid
	msg []byte
}

// mockProcessor is a thread-safe BidProcessor for use in tests.
type mockProcessor struct {
	mu        sync.Mutex
	processFn func(ctx context.Context, bid auction.Bid, msg []byte) error
	calls     []processorCall
}

func (m *mockProcessor) ProcessBid(ctx context.Context, bid auction.Bid, msg []byte) error {
	m.mu.Lock()
	m.calls = append(m.calls, processorCall{bid: bid, msg: msg})
	m.mu.Unlock()

	if m.processFn != nil {
		return m.processFn(ctx, bid, msg)
	}

	return nil
}

func (m *mockProcessor) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.calls)
}

func (m *mockProcessor) firstCall() processorCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.calls[0]
}

// discardLogger returns a logger that drops all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newTestServer builds an httptest.Server from an auction.Handler and registers
// t.Cleanup(srv.Close).
func newTestServer(t *testing.T, proc auction.BidProcessor) (*httptest.Server, *hub.Hub) {
	t.Helper()

	h := hub.NewHub()
	auctionHandler := auction.NewHandler(proc, "", h, discardLogger())
	mux := http.NewServeMux()
	auctionHandler.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, h
}

// dialWS dials the WebSocket endpoint for the given auctionID on srv.
// The connection is closed via t.Cleanup.
func dialWS(t *testing.T, srv *httptest.Server, auctionID string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + srv.URL[len("http"):] + "/api/v1/ws/auction/" + auctionID
	conn, resp, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err, "dial WebSocket")

	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn
}

// sendMsg writes a text message to conn.
func sendMsg(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	err := conn.Write(ctx, websocket.MessageText, []byte(payload))
	require.NoError(t, err, "write WebSocket message")
}
