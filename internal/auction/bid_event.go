package auction

import "time"

// BidEvent wraps a Bid with metadata about when it was received by the system.
// It is used to enqueue bids for asynchronous persistence by the worker.
// ReceivedAt enables tracking processing latency and debugging.
type BidEvent struct {
	Bid        Bid
	ReceivedAt time.Time
}
