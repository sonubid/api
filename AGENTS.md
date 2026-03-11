# AGENTS.md — SonuBid API

Guide for code agents working in this repository.
All code and documentation must be in **English**.

---

## Project overview

Real-time auction/bidding API built in Go. Browsers connect via WebSocket and
receive live bid updates. The system is split into three layers:

```
Browser ──WebSocket──► Handler (hub)
                            │
                            ▼
                       Processor          ← validates bids in-memory (Store)
                            │
                      ┌─────┴──────┐
                      │            │
                  Broadcast      Enqueue
                      │            │
                 Hub.send       Queue (chan)
                   channel           │
                      │            Worker
                      ▼              │
                   Browser        Repository
                                     │
                                  PostgreSQL
```

- **Hub** — manages WebSocket connections grouped by auction room.
- **Processor** — validates incoming bids against the in-memory Store; calls
  Broadcast on success and enqueues the event for persistence.
- **Worker** — background goroutine that drains the Queue and persists bids via
  the Repository.
- **Store** — in-memory auction state (`sync.RWMutex`).
- **Repository** — PostgreSQL persistence (`pgx`).
- **Queue** — internal Go channel (`chan BidEvent`).

---

## Tech stack

| Concern | Choice |
|---|---|
| Language | Go 1.26 |
| HTTP | `net/http` (stdlib) |
| WebSocket | `github.com/coder/websocket` |
| In-memory store | `sync.RWMutex` |
| Database | PostgreSQL via `pgx` |
| Tests | `github.com/stretchr/testify` (suite pattern) |
| Linter | `golangci-lint` |
| Pre-commit hooks | `lefthook` |

---

## Commands

```bash
# Build
go build ./...

# Run all tests with race detector
go test -race ./...

# Run tests for a single package
go test -race ./internal/hub/...

# Run a single test by name
go test -race -run TestHubSuite/TestBroadcastMessageReachesAllClientsInRoom ./internal/hub/...

# Coverage report
go test -race -coverprofile=cover.out ./...
go tool cover -func=cover.out

# Lint (must pass with 0 issues before finishing any feature)
golangci-lint run ./...

# Lint a single package
golangci-lint run ./internal/hub/...

# Vulnerability check
govulncheck ./...

# Install lefthook
lefthook install

# Pre-commit hook (runs lint + vulncheck + tests automatically)
lefthook run pre-commit
```

---

## Package structure

- One directory = one package. No sub-packages purely for organisation.
- Multiple `.go` files per package are expected and encouraged.
- One domain type per file (`auction.go`, `bid.go`, `state.go`, etc.).
- All packages live under `internal/`.
- Interfaces live in the `auction` package (consumer packages import them).

Current layout:

```
cmd/api/       # main.go — wires everything together (Feature 8 - pending)
internal/
  auction/     # domain models + interfaces (Feature 1 — complete)
  hub/         # WebSocket hub + client + HTTP handler (Feature 2 — complete)
  store/       # Store implementation — sync.RWMutex (Feature 3 - complete)
  processor/   # bid validation + broadcast + enqueue (Feature 4 - complete)
  queue/       # Queue implementation — chan BidEvent (Feature 5 - complete)
  worker/      # background persistence goroutine (Feature 6 - complete)
  repository/  # Repository implementation — pgx (Feature 7 - pending)
```

---

## Code style

### Formatting & imports
- Standard `gofmt` / `goimports` formatting. No exceptions.
- Import groups (separated by blank lines):
  1. stdlib
  2. third-party
  3. internal (`github.com/sonubid/api/...`)

### Documentation
- Every exported type, function, method, and constant must have a Go doc comment.
- Package-level doc comment required in every package (on the `package` line file).
- No inline comments inside function bodies — code must be self-explanatory.
- Doc comments are written in English, in full sentences, starting with the name
  of the symbol being documented.

### Naming
- Follow standard Go naming: `MixedCaps` for exported, `mixedCaps` for unexported.
- Acronyms: `ID`, `URL`, `HTTP`, `WS` — never `Id`, `Url`, `Http`.
- Avoid redundant package prefixes: `auction.State` not `auction.AuctionState`.
- Interface names: single-method interfaces end in `-er` (`Store`, `Queue`,
  `Repository` are acceptable domain terms).
- Test suite types: `hubSuite`, `clientSuite`, `handlerSuite`.

### Types
- Money / bid amounts: `uint64` (cents, no decimals, no negatives).
- User identity: `string` (no `User` struct in the auction domain).
- IDs: `string`.

### Error handling
- Return `error` as the last value; never panic in production paths.
- Sentinel errors in `errors.go` per package using `errors.New`.
- Wrap errors with context: `fmt.Errorf("processor: %w", err)`.
- Never discard errors silently. Use `_ = expr` when intentionally ignoring
  a return value and add a comment if the reason is non-obvious.
- Do not use `//nolint:errcheck` — use `_ =` instead.

### Concurrency
- Mutex fields first in struct definitions, unless struct padding interferes.
- Release locks before performing I/O (never hold a lock across a network call).
- Use `context.Context` derived from the request/caller, never `context.Background()`
  in production code paths.

---

## Testing conventions

### General rules
- Minimum **85% coverage** per package. Aim for 95%+.
- Always run tests with `-race`.
- Use the **suite pattern** from `testify/suite` for all test files.
- Use **subtests** for table-driven cases within a suite method.
- Black-box tests in `package foo_test`; white-box tests (needing unexported
  access) in `package foo`. Document why white-box access is needed.
- Use `export_test.go` (in `package foo`) to expose unexported symbols to
  `package foo_test` when needed.

### Naming
- Test function names use **no underscores**: `TestBroadcastMessageReachesAllClients`,
  not `TestBroadcast_MessageReachesAllClients`.
- Suite runner: `TestXxxSuite` (e.g. `TestHubSuite`).
- Test methods on suite: `TestVerbNounDescription` in plain CamelCase.

### Constants
- Define constants for any string or numeric literal that appears **more than
  once** across test files in the same package. This avoids SonarQube
  duplication warnings.

```go
const (
    auctionOne  = "auction-1"
    auctionTwo  = "auction-2"
    bidPayload  = `{"bid":100}`
    waitTimeout = 2 * time.Second
)
```

### Helpers
- Shared test helpers go in `helpers_test.go`.
- Use `require.Eventually` instead of manual polling loops.
- Goroutines spawned in test helpers must be bound to a context cancelled via
  `t.Cleanup(cancel)` to avoid goroutine leaks on test failure.
- Use `t.Helper()` in all helper functions.

---

## Workflow for each feature

1. Implement the feature code.
2. Write tests — minimum 85% coverage, suite pattern, no underscores in names.
3. Run `golangci-lint run ./...` — **must be 0 issues** before proceeding.
4. Run `go test -race ./...` — **must be green** before proceeding.
5. Launch a **code review sub-agent** to review all changed files. Address every
   finding before marking the feature complete.
6. Only commit after linter + tests + code review are all clean.

---

## Linters enabled (golangci-lint)

`gosec`, `bodyclose`, `noctx`, `revive`, `gocritic`, `misspell`, `whitespace`,
`unconvert`, `tagalign`, `predeclared`, `modernize`, `sloglint`, `usestdlibvars`,
`perfsprint`, `prealloc`, `errorlint`, `errname`, `nilnil`, `nilerr`, `nestif`,
`mnd`, `copyloopvar`, `gocyclo`, `gocognit`, `ireturn`, `iotamixing`, `iface`,
`godoclint`, `funcorder`, `embeddedstructfieldcheck`.

The `mnd` linter flags magic numbers — define named constants instead.
The `godoclint` linter enforces Go doc comment format on all exported symbols.

---

## WebSocket notes

- Library: `github.com/coder/websocket` (formerly `nhooyr.io/websocket`).
- `conn.CloseNow()` is immediate/unilateral — use in cleanup paths and defers.
- `conn.Close()` performs the full closing handshake — use when the local side
  initiates a clean close.
- `websocket.Dial()` returns `(*Conn, *http.Response, error)` — the response
  body must be closed: `if resp != nil && resp.Body != nil { resp.Body.Close() }`.
- In production, pass `opts.OriginPatterns` to `websocket.Accept` to restrict
  cross-origin connections. Never ship `InsecureSkipVerify: true` in production
  handlers — inject `*websocket.AcceptOptions` as a parameter instead.
- `Handler` in `internal/hub` derives its context from `r.Context()` so that
  server shutdown propagates to all active WebSocket sessions.
