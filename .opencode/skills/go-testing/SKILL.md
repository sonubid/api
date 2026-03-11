---
name: go-testing
description: Testing conventions for Go packages using testify suite pattern, naming rules, helpers, and coverage requirements
license: MIT
compatibility: opencode
---

## When to use this skill

Load this skill whenever you are writing, modifying, or reviewing test files in any package under `internal/`.

---

## General rules

- Minimum **85% coverage** per package. Aim for 95%+.
- Always run tests with `-race`: `go test -race ./...`
- Use the **suite pattern** from `testify/suite` for all test files.
- Use **subtests** for table-driven cases within a suite method.
- Black-box tests go in `package foo_test`. White-box tests (needing unexported access) go in `package foo` — document why white-box access is needed.
- Use `export_test.go` (in `package foo`) to expose unexported symbols to `package foo_test` when needed.

---

## Naming

- Suite runner function: `TestXxxSuite` — e.g. `TestHubSuite`, `TestProcessorSuite`.
- Suite struct: lowercase camel — e.g. `hubSuite`, `processorSuite`.
- Test method names: **no underscores**, plain CamelCase — `TestBroadcastMessageReachesAllClientsInRoom`, not `TestBroadcast_MessageReachesAllClients`.
- Pattern: `TestVerbNounDescription`.

---

## Constants

Define constants for any string or numeric literal that appears **more than once** across test files in the same package. This avoids SonarQube duplication warnings.

```go
const (
    auctionOne  = "auction-1"
    auctionTwo  = "auction-2"
    bidPayload  = `{"bid":100}`
    waitTimeout = 2 * time.Second
)
```

---

## Helpers

- Shared test helpers go in `helpers_test.go`.
- Use `require.Eventually` instead of manual polling loops.
- Goroutines spawned in test helpers must be bound to a context cancelled via `t.Cleanup(cancel)` to avoid goroutine leaks on test failure.
- Always call `t.Helper()` at the top of every helper function.

---

## Suite boilerplate

```go
package foo_test

import (
    "testing"

    "github.com/stretchr/testify/suite"
)

type fooSuite struct {
    suite.Suite
}

func TestFooSuite(t *testing.T) {
    suite.Run(t, new(fooSuite))
}

func (s *fooSuite) TestVerbNounDescription() {
    // ...
}
```

---

## After writing tests

Run:

```bash
go test -race ./internal/<package>/...
go test -race -coverprofile=cover.out ./internal/<package>/...
go tool cover -func=cover.out
```

Coverage must be ≥85% before the feature is considered complete.
