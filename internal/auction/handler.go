package auction

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/sonubid/api/internal/hub"
)

// BidProcessor is the narrow interface that the auction handler requires to
// validate and broadcast incoming bids.
type BidProcessor interface {
	ProcessBid(ctx context.Context, bid Bid, msg []byte) error
}

// Handler serves auction WebSocket routes and delegates bid processing.
type Handler struct {
	proc          BidProcessor
	allowedOrigin string
	hub           *hub.Hub
	logger        *slog.Logger
}

// NewHandler returns a new auction route handler.
//
// AllowedOrigin controls WebSocket origin validation. When it is empty, all
// origins are accepted by enabling InsecureSkipVerify and a warning is logged.
// This mode is intended for local development only.
func NewHandler(proc BidProcessor, allowedOrigin string, h *hub.Hub, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}

	if allowedOrigin == "" {
		logger.Warn("AllowedOrigin is empty: enabling InsecureSkipVerify; websocket origin checking disabled, all origins accepted")
	}

	return &Handler{
		proc:          proc,
		allowedOrigin: allowedOrigin,
		hub:           h,
		logger:        logger,
	}
}

// RegisterRoutes registers all auction HTTP routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/ws/auction/{auctionID}", h.wsHandler())
}

// BidRequest is the JSON payload sent by a browser client to place a bid.
// AuctionID is inferred from the WebSocket URL path and is not part of this payload.
type BidRequest struct {
	// UserID identifies the bidder.
	UserID string `json:"userId"`
	// Amount is the bid value in cents.
	Amount uint64 `json:"amount"`
}

// BidResponse is the JSON payload broadcast to all clients in an auction room
// when a bid is accepted. It carries the server-assigned ID and timestamp
// alongside the bidder and amount fields.
type BidResponse struct {
	// AuctionID is the identifier of the auction this bid belongs to.
	AuctionID string `json:"auctionId"`
	// UserID identifies the bidder.
	UserID string `json:"userId"`
	// Amount is the bid value in cents.
	Amount uint64 `json:"amount"`
}

func (h *Handler) wsHandler() http.HandlerFunc {
	opts := &websocket.AcceptOptions{}
	if h.allowedOrigin == "" {
		opts.InsecureSkipVerify = true
	} else {
		opts.OriginPatterns = []string{h.allowedOrigin}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		auctionID := r.PathValue("auctionID")
		msgHandler := h.makeMsgHandler(r.Context(), auctionID)
		hub.ServeAuctionWS(h.hub, auctionID, msgHandler, opts)(w, r)
	}
}

func (h *Handler) makeMsgHandler(ctx context.Context, auctionID string) func(msg []byte) {
	return func(msg []byte) {
		var req BidRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			h.logger.Warn("invalid bid message",
				slog.String("auction_id", auctionID),
				slog.Any("error", err))
			return
		}

		now := time.Now()
		bid := Bid{
			ID:        uuid.NewString(),
			AuctionID: auctionID,
			UserID:    req.UserID,
			Amount:    req.Amount,
			PlacedAt:  now,
		}

		resp := BidResponse{
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
