package auction

// Broadcaster defines the contract for broadcasting messages to all clients
// connected to a given auction room. Implementations must be safe for
// concurrent use.
type Broadcaster interface {
	// Broadcast sends a message to every client currently registered in the
	// auction identified by auctionID. Clients that cannot receive the
	// message immediately are skipped to avoid blocking the caller.
	Broadcast(auctionID string, message []byte)
}
