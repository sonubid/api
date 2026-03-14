// Package handler wires all domain HTTP handlers onto a single ServeMux and
// exposes them as an http.Handler ready to be passed to server.Start.
//
// Each domain has its own file (auction.go, …) that defines an unexported
// handler struct and a register method. New instantiates every domain handler
// and calls register so that callers only ever interact with the top-level
// Config and New function.
package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/hub"
)

// BidProcessor is the narrow interface that the auction handler requires to
// validate and broadcast incoming bids.
type BidProcessor interface {
	ProcessBid(ctx context.Context, bid auction.Bid, msg []byte) error
}

// Config groups all dependencies required to build the full HTTP handler tree.
// Add a new field here whenever a new domain handler introduces a dependency.
// Fields are ordered from largest to smallest alignment to minimise struct padding.
type Config struct {
	// Processor handles bid validation and broadcasting.
	Processor BidProcessor
	// AllowedOrigin is the CORS origin pattern accepted by the WebSocket
	// upgrade. An empty string disables origin checking (development only).
	AllowedOrigin string
	// Hub is the WebSocket connection manager.
	Hub *hub.Hub
	// Logger is used for structured logging across all handlers.
	Logger *slog.Logger
}

// New registers all domain handlers onto a fresh ServeMux and returns the mux
// as an http.Handler.
func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	newAuctionHandler(cfg).register(mux)
	return mux
}
