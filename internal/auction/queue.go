package auction

// Queue defines the contract for enqueuing bid events for asynchronous
// processing. It decouples the real-time bid processing from the persistence
// worker, allowing the system to handle high throughput without blocking.
type Queue interface {
	// Enqueue adds a bid event to the queue for later processing.
	// Returns an error if the enqueue operation fails.
	Enqueue(event BidEvent) error

	// Events returns a channel that emits bid events in the order they
	// were enqueued. The channel is closed when the queue is shut down.
	Events() <-chan BidEvent

	// Close shuts down the queue and closes the Events channel, signalling
	// consumers that no more events will arrive. It must be called by the
	// owner (e.g. main) after all producers have stopped enqueueing.
	// Implementations must be safe to call more than once. Close intentionally
	// returns no error; implementations that cannot fail (e.g. an in-memory
	// channel) have no error to report.
	Close()
}
