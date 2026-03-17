package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/sonubid/api/internal/auction"
	"github.com/sonubid/api/internal/dto"
	"github.com/sonubid/api/internal/hub"
)

// auctionHandler holds the dependencies for all auction-related HTTP routes.
// Fields are ordered from largest to smallest alignment to minimise struct padding.
type auctionHandler struct {
	proc          BidProcessor
	allowedOrigin string
	hub           *hub.Hub
	logger        *slog.Logger
}

// newAuctionHandler constructs an auctionHandler from the shared Config.
// When cfg.AllowedOrigin is empty, WebSocket origin checking is disabled; a
// warning is logged so operators are aware that all origins are accepted.
func newAuctionHandler(cfg Config) *auctionHandler {
	if cfg.AllowedOrigin == "" {
		cfg.Logger.Warn("AllowedOrigin is empty: websocket origin checking disabled, all origins accepted")
	}

	return &auctionHandler{
		proc:          cfg.Processor,
		allowedOrigin: cfg.AllowedOrigin,
		hub:           cfg.Hub,
		logger:        cfg.Logger,
	}
}

// register mounts all auction routes onto mux.
func (h *auctionHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/ws/auction/{auctionID}", h.wsHandler())
}

// wsHandler returns an http.HandlerFunc that upgrades the connection to
// WebSocket for the auction room identified by {auctionID} and wires
// incoming messages to the processor.
// allowedOrigin is captured once at registration time.
func (h *auctionHandler) wsHandler() http.HandlerFunc {
	opts := &websocket.AcceptOptions{
		OriginPatterns: []string{h.allowedOrigin},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		auctionID := r.PathValue("auctionID")
		msgHandler := h.makeMsgHandler(r.Context(), auctionID)
		hub.Handler(h.hub, auctionID, msgHandler, opts)(w, r)
	}
}

// makeMsgHandler returns a closure that parses a raw WebSocket message as a
// BidRequest DTO, maps it to a domain Bid, and delegates to the BidProcessor.
// Malformed messages are logged and silently dropped. Rejected bids are logged
// at Warn level.
func (h *auctionHandler) makeMsgHandler(ctx context.Context, auctionID string) func([]byte) {
	return func(raw []byte) {
		var req dto.BidRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			h.logger.Warn("invalid bid message",
				slog.String("auction_id", auctionID),
				slog.Any("error", err))
			return
		}

		now := time.Now()
		bid := auction.Bid{
			ID:        uuid.NewString(),
			AuctionID: auctionID,
			UserID:    req.UserID,
			Amount:    req.Amount,
			PlacedAt:  now,
		}

		resp := dto.BidResponse{
			AuctionID: bid.AuctionID,
			UserID:    bid.UserID,
			Amount:    bid.Amount,
		}

		broadcastMsg, err := json.Marshal(resp)
		if err != nil {
			h.logger.Error("failed to marshal bid response",
				slog.String("auction_id", auctionID),
				slog.Any("error", err))

			return
		}

		if err := h.proc.ProcessBid(ctx, bid, broadcastMsg); err != nil {
			h.logger.Warn("bid rejected",
				slog.String("auction_id", auctionID),
				slog.String("user_id", req.UserID),
				slog.Uint64("amount", req.Amount),
				slog.Any("error", err))
		}
	}
}
