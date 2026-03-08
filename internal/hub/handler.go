package hub

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
)

// Handler returns an http.HandlerFunc that upgrades incoming HTTP requests to
// WebSocket connections and integrates them into the Hub for the auction
// identified by auctionID.
//
// On each new connection the handler:
//  1. Accepts the WebSocket upgrade using the provided opts (pass nil for defaults).
//  2. Creates a Client and registers it with the Hub.
//  3. Starts the client's WritePump and ReadPump as goroutines.
//  4. Unregisters and cleans up the client when either pump exits.
//
// The msgHandler parameter receives every raw message sent by the client and
// is called from the ReadPump goroutine. Callers typically wire this to the
// bid processor so that incoming bid messages are validated and broadcast.
//
// opts is passed directly to websocket.Accept. Production callers should set
// opts.OriginPatterns to restrict cross-origin connections; passing nil uses
// the library defaults.
func Handler(h *Hub, auctionID string, msgHandler func([]byte), opts *websocket.AcceptOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, opts)
		if err != nil {
			return
		}

		c := NewClient(conn)
		h.Register(auctionID, c)

		ctx, cancel := context.WithCancel(r.Context())

		go func() {
			c.WritePump(ctx)
			cancel()
		}()

		c.ReadPump(ctx, msgHandler)

		cancel()
		h.Unregister(auctionID, c)
	}
}
