# AGENTS.md — SonuBid API

Guide for code agents working in this repository.
All code and documentation must be in **English**.

---

## Skills

This project uses OpenCode skills for on-demand instructions. Load them when needed:

| Skill | When to load |
|---|---|
| `go-code-style` | Writing new Go code or refactoring |
| `go-testing` | Writing or modifying test files |
| `websocket` | Working in `internal/hub` or any WebSocket code |
| `code-review` | Step 5 of the feature workflow (always required) |

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

## Linters enabled (golangci-lint)

`gosec`, `bodyclose`, `noctx`, `revive`, `gocritic`, `misspell`, `whitespace`,
`unconvert`, `tagalign`, `predeclared`, `modernize`, `sloglint`, `usestdlibvars`,
`perfsprint`, `prealloc`, `errorlint`, `errname`, `nilnil`, `nilerr`, `nestif`,
`mnd`, `copyloopvar`, `gocyclo`, `gocognit`, `ireturn`, `iotamixing`, `iface`,
`godoclint`, `funcorder`, `embeddedstructfieldcheck`.

The `mnd` linter flags magic numbers — define named constants instead.
The `godoclint` linter enforces Go doc comment format on all exported symbols.

---

## Workflow for each feature

1. Load skill `go-code-style` before writing any code.
2. Implement the feature code.
3. Load skill `go-testing` before writing tests. Write tests — minimum 85% coverage.
4. If working in `internal/hub` or any WebSocket code, load skill `websocket`.
5. Run `golangci-lint run ./...` — **must be 0 issues** before proceeding.
6. Run `go test -race ./...` — **must be green** before proceeding.
7. Load skill `code-review` and launch a **code review sub-agent** to review all
   changed files. Address every finding before proceeding.
8. Only commit after linter + tests + code review are all clean.
