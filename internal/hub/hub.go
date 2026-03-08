// Package hub manages WebSocket connections grouped by auction.
//
// It provides a Hub that tracks active clients per auction and broadcasts
// messages to all connected clients in a given auction room. It also exposes
// a Client type that wraps a single WebSocket connection and handles
// concurrent read and write pumps, and an HTTP handler that upgrades
// incoming requests to WebSocket connections.
package hub

import (
	"sync"
)

// Hub maintains the set of active clients grouped by auction ID and
// broadcasts messages to all clients within a given auction room.
// All methods are safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{}
}

// NewHub creates and returns an initialized Hub ready to accept clients.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*Client]struct{}),
	}
}

// Register adds a client to the set of active clients for the given auction.
// If no room exists for the auction yet, it is created automatically.
func (h *Hub) Register(auctionID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[auctionID] == nil {
		h.clients[auctionID] = make(map[*Client]struct{})
	}

	h.clients[auctionID][c] = struct{}{}
}

// Unregister removes a client from the set of active clients for the given
// auction and closes its WebSocket connection. If the auction room becomes
// empty after removal, the room entry is deleted from the map.
func (h *Hub) Unregister(auctionID string, c *Client) {
	h.mu.Lock()

	room, ok := h.clients[auctionID]
	if ok {
		delete(room, c)
		if len(room) == 0 {
			delete(h.clients, auctionID)
		}
	}

	h.mu.Unlock()

	if ok {
		_ = c.conn.CloseNow()
	}
}

// Broadcast sends msg to every client currently registered in the given
// auction room. Clients whose send channel is full are skipped to avoid
// blocking the broadcast for all other clients.
func (h *Hub) Broadcast(auctionID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients[auctionID] {
		select {
		case c.send <- msg:
		default:
		}
	}
}

// ClientCount returns the number of clients currently registered in the
// given auction room. It is primarily intended for observability and testing.
func (h *Hub) ClientCount(auctionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients[auctionID])
}
