---
name: go-code-style
description: Go code style conventions for formatting, imports, naming, types, error handling, concurrency, and documentation
license: MIT
compatibility: opencode
---

## When to use this skill

Load this skill when writing new Go code or refactoring existing code in any package under `internal/` or `cmd/`.

---

## Formatting & imports

- Use standard `gofmt` / `goimports` formatting. No exceptions.
- Imports must be in **3 groups**, separated by blank lines:
  1. stdlib
  2. third-party
  3. internal (`github.com/sonubid/api/...`)

```go
import (
    "context"
    "fmt"

    "github.com/coder/websocket"

    "github.com/sonubid/api/internal/auction"
)
```

---

## Naming

- Exported identifiers: `MixedCaps`. Unexported: `mixedCaps`.
- Acronyms are always uppercase: `ID`, `URL`, `HTTP`, `WS` — never `Id`, `Url`, `Http`.
- Avoid redundant package prefixes: `auction.State` not `auction.AuctionState`.
- Single-method interfaces end in `-er` where natural. Domain terms (`Store`, `Queue`, `Repository`) are acceptable as-is.

---

## Types

- Money and bid amounts: `uint64` (cents — no decimals, no negatives).
- User identity: `string` (no `User` struct in the auction domain).
- IDs: `string`.

---

## Documentation

- Every exported type, function, method, and constant must have a Go doc comment.
- Every file must have a package-level doc comment on the `package` line.
- Doc comments start with the name of the symbol: `// Hub manages WebSocket connections...`
- Doc comments are full sentences in English.
- No inline comments inside function bodies — code must be self-explanatory.

---

## Error handling

- `error` is always the last return value.
- Never `panic` in production paths.
- Sentinel errors live in `errors.go` per package, defined with `errors.New`:

```go
var ErrAuctionNotFound = errors.New("auction not found")
```

- Wrap errors with context using `%w`:

```go
return fmt.Errorf("processor: validate bid: %w", err)
```

- Never discard errors silently. Use `_ = expr` when intentional; add a comment if the reason is non-obvious.
- Do not use `//nolint:errcheck` — use `_ =` instead.

---

## Concurrency

- Mutex fields appear **first** in struct definitions, unless struct padding requires otherwise.
- Never hold a lock across I/O or network calls — release before performing I/O.
- Always use `context.Context` derived from the request/caller. Never use `context.Background()` in production code paths.

```go
// Correct
func (s *Store) Update(ctx context.Context, id string, amount uint64) error {
    s.mu.Lock()
    s.data[id] = amount
    s.mu.Unlock()         // lock released before any I/O below
    return s.notify(ctx, id)
}
```

---

## Struct field ordering

Order struct fields **from largest to smallest alignment** to minimise padding bytes inserted by the compiler. On a 64-bit platform:

| Size | Types |
|---|---|
| 16 bytes | interfaces, `string` |
| 8 bytes | pointers (`*T`), `int64`, `uint64`, `float64`, slices, maps, channels |
| 4 bytes | `int32`, `uint32`, `float32` |
| 2 bytes | `int16`, `uint16` |
| 1 byte | `int8`, `uint8`, `bool` |

Exception: `sync.Mutex` / `sync.RWMutex` always go **first**, regardless of size, to avoid accidental copying and to satisfy the concurrency rule.

```go
// Correct — 16-byte fields first, then 8-byte pointers
type auctionHandler struct {
    proc          BidProcessor  // interface, 16 bytes
    allowedOrigin string        // 16 bytes
    hub           *hub.Hub      // pointer, 8 bytes
    logger        *slog.Logger  // pointer, 8 bytes
}

// Incorrect — mixed sizes cause unnecessary padding
type auctionHandler struct {
    hub           *hub.Hub      // 8 bytes — padding hole here
    proc          BidProcessor  // 16 bytes
    logger        *slog.Logger  // 8 bytes — padding hole here
    allowedOrigin string        // 16 bytes
}
```

---

## Magic numbers

Define named constants for any numeric or string literal that is not immediately obvious. The `mnd` linter will flag violations.

```go
const (
    // MinBidAmount is the minimum valid bid in cents.
    MinBidAmount uint64 = 100
)
```
