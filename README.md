# SonuBid API

Real-time auction and bidding backend. Browsers connect via WebSocket to place bids and receive live updates as other participants bid in the same auction room.

## How it works

Each auction has a dedicated room identified by its ID. Clients connect to the room's WebSocket endpoint, send bid messages, and receive broadcast notifications every time a bid is accepted. Bid validation happens in memory — bids must exceed the current highest bid (or the starting price if no bids have been placed yet). Accepted bids are enqueued and persisted asynchronously by a pool of background workers, keeping the hot path fast.

```
Browser ──WebSocket──► Handler
                           │
                           ▼
                       Processor   ← validates bids (Store)
                           │
                   ┌───────┴────────┐
                   │                │
               Broadcast          Enqueue
                   │                │
              Hub (send)         Queue (chan)
                   │                │
               Browsers           Worker
                                    │
                                Repository
```

## Requirements

- Go 1.26 or later
- [`golangci-lint`](https://golangci-lint.run/usage/install/) (for linting)
- [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) (for vulnerability scanning)
- [`lefthook`](https://github.com/evilmartians/lefthook) (for pre-commit hooks, optional)

## Installation

```bash
git clone https://github.com/sonubid/api.git
cd api
go mod download
```

## Running

```bash
go run ./cmd/api
```

The server listens on `:8080` by default.

## Configuration

| Variable | Description | Default |
|---|---|---|
| `ALLOWED_ORIGIN` | Allowed WebSocket origin pattern. Accepts wildcards (e.g. `https://*.example.com`). Empty string enables `InsecureSkipVerify` (development only). | `""` |

## WebSocket protocol

### Connect

```
ws://localhost:8080/api/v1/ws/auction/{auctionID}
```

Replace `{auctionID}` with the target auction's identifier (e.g. `auction-1`).

### Place a bid

Send a JSON text message:

```json
{
  "userId": "user-42",
  "amount": 1500
}
```

`amount` is expressed in **cents** (integer).

### Receive updates

When a bid is accepted, the server broadcasts the following message to every connected client in the room:

```json
{
  "auctionId": "auction-1",
  "userId": "user-42",
  "amount": 1500
}
```

Rejected bids (amount too low, auction not found) generate no response to the sender and no broadcast.

## Testing

```bash
# Run all tests with the race detector
go test -race ./...

# Run tests for a single package
go test -race ./internal/hub/...

# Run a single test by name
go test -race -run TestHubSuite/TestBroadcastMessageReachesAllClientsInRoom ./internal/hub/...

# Coverage report
go test -race -coverprofile=cover.out ./...
go tool cover -func=cover.out
```

## Linting

```bash
# Full lint (must report 0 issues)
golangci-lint run ./...

# Single package
golangci-lint run ./internal/hub/...

# Vulnerability check
govulncheck ./...
```

## Pre-commit hooks

Install [lefthook](https://github.com/evilmartians/lefthook) and run:

```bash
lefthook install
```

Each commit will automatically run the linter, vulnerability check, and short tests in parallel.

## Package layout

```
cmd/api/          # entry point — wires all components
internal/
  auction/        # domain types and auction feature routes
  hub/            # WebSocket connection manager
  store/          # in-memory auction state (sync.RWMutex)
  processor/      # bid validation, broadcast, and enqueue
  queue/          # event queue (buffered channel)
  worker/         # background persistence goroutines
  repository/     # bid storage
  server/         # lifecycle-managed HTTP server
```

## Design conventions

- `cmd/api` is the composition root: it builds dependencies, creates `http.ServeMux`, and starts the server.
- Features own their transport routes. Each feature registers endpoints via `RegisterRoutes(*http.ServeMux)`.
- Avoid generic technical-layer packages (`handler`, `service`, `dto`, `util`) used only for organisation, as in typical Java/C# layering.
  Keep route logic and wire models in the feature package that owns the use case.
- Interfaces are defined where they are consumed (small, local contracts), not in a central package.
- Providers (`store`, `repository`, `queue`) expose concrete implementations; interface conformance is validated at wiring call sites by compilation.

## License

Apache 2.0 — see [LICENSE](LICENSE).
