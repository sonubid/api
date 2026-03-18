package hub_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/sonubid/api/internal/hub"
)

const (
	waitTimeout  = 2 * time.Second
	pollInterval = 10 * time.Millisecond
)

// testAcceptOpts returns AcceptOptions suitable for use in tests. It disables
// the origin check so that connections from httptest servers are accepted.
func testAcceptOpts() *websocket.AcceptOptions {
	return &websocket.AcceptOptions{InsecureSkipVerify: true}
}

// newHub is a convenience wrapper so test files don't need to import hub.NewHub
// directly, keeping the helper centralized.
func newHub() *hub.Hub {
	return hub.NewHub()
}

// newTestPair starts an httptest.Server using hub.Handler for the given
// auctionID and dials it, returning the server and the client-side connection.
// If received is non-nil, every message delivered to the client via Broadcast
// is forwarded into that channel. The read goroutine is bound to the test
// lifetime via t.Cleanup so it exits when the connection is closed.
func newTestPair(
	t *testing.T,
	h *hub.Hub,
	auctionID string,
	received chan []byte,
) (*httptest.Server, *websocket.Conn) {
	t.Helper()

	srv := httptest.NewServer(hub.Handler(h, auctionID, func(_ []byte) {
		// No-op handler since tests that use newTestPair don't care about client messages.
	}, testAcceptOpts()))
	t.Cleanup(srv.Close)

	// Capture count before dialing so we can wait for exactly +1 regardless
	// of how many clients are already registered in the room.
	before := h.ClientCount(auctionID)

	wsURL := "ws" + srv.URL[len("http"):]
	conn, resp, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err, "dial websocket")
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	if received != nil {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		go func() {
			for {
				_, msg, readErr := conn.Read(ctx)
				if readErr != nil {
					return
				}
				received <- msg
			}
		}()
	}

	// Wait for the server-side handler goroutine to register the client.
	waitForCount(t, h, auctionID, before+1)

	return srv, conn
}

// waitForCount blocks until h.ClientCount(auctionID) equals want or the
// 2-second timeout elapses.
func waitForCount(t *testing.T, h *hub.Hub, auctionID string, want int) {
	t.Helper()
	require.Eventually(
		t,
		func() bool { return h.ClientCount(auctionID) == want },
		waitTimeout,
		pollInterval,
		"hub.ClientCount(%q): want %d", auctionID, want,
	)
}

// waitMsg returns the first message received on ch within 2 seconds.
func waitMsg(t *testing.T, ch chan []byte) []byte {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for message")
		return nil
	}
}

// drainNonBlocking reads a single message from ch without blocking.
// Returns nil if no message is available immediately.
func drainNonBlocking(ch chan []byte) []byte {
	select {
	case msg := <-ch:
		return msg
	default:
		return nil
	}
}

// bidPayload returns a simple JSON bid message string for the given index,
// used to generate unique messages in broadcast-drop tests.
func bidPayload(i int) string {
	return fmt.Sprintf(`{"bid":%d}`, i)
}
