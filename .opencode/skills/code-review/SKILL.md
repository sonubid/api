---
name: code-review
description: Code review checklist for Go packages covering style, error handling, concurrency, docs, tests, lint, and security
license: MIT
compatibility: opencode
---

## When to use this skill

Load this skill at **step 5 of the feature workflow**: after implementing the feature and passing lint + tests, before committing. Review every file changed in the feature.

---

## How to perform the review

Go through each checklist section for every changed file. Report every finding with: file path, line number, issue, and suggested fix. Address all findings before marking the feature complete.

---

## Checklist: Code style

- [ ] Imports are in 3 groups separated by blank lines: stdlib / third-party / internal (`github.com/sonubid/api/...`)
- [ ] No `gofmt`/`goimports` violations
- [ ] Exported names use `MixedCaps`; unexported use `mixedCaps`
- [ ] Acronyms are uppercase: `ID`, `URL`, `HTTP`, `WS` — never `Id`, `Url`, `Http`
- [ ] No redundant package prefixes: `auction.State` not `auction.AuctionState`
- [ ] Money/bid amounts use `uint64`; user identity and IDs use `string`
- [ ] No magic numbers — named constants defined instead

---

## Checklist: Documentation

- [ ] Every exported type, function, method, and constant has a Go doc comment
- [ ] Package-level doc comment present in every file with `package` declaration
- [ ] Doc comments start with the name of the symbol being documented
- [ ] Doc comments are full sentences in English
- [ ] No inline comments inside function bodies

---

## Checklist: Error handling

- [ ] `error` is the last return value
- [ ] No `panic` in production paths
- [ ] Sentinel errors defined in `errors.go` using `errors.New`
- [ ] Errors wrapped with context: `fmt.Errorf("package: %w", err)`
- [ ] No silently discarded errors — use `_ = expr` with a comment if intentional
- [ ] No `//nolint:errcheck` — use `_ =` instead

---

## Checklist: Concurrency

- [ ] Mutex fields appear first in struct definitions (unless struct padding requires otherwise)
- [ ] No locks held across I/O or network calls
- [ ] `context.Context` is derived from the request/caller — no `context.Background()` in production paths

---

## Checklist: Tests

- [ ] Coverage ≥ 85% for the package (run `go test -race -coverprofile=cover.out ./internal/<pkg>/...` + `go tool cover -func=cover.out`)
- [ ] All tests pass with `-race`
- [ ] Suite pattern used (`testify/suite`)
- [ ] Test method names: no underscores, plain CamelCase (`TestVerbNounDescription`)
- [ ] String/numeric literals repeated more than once are extracted as constants
- [ ] Helpers use `t.Helper()` and are in `helpers_test.go`

---

## Checklist: Linters

Run `golangci-lint run ./internal/<pkg>/...` and verify **0 issues**. Pay special attention to:

- `mnd` — magic numbers not extracted as constants
- `godoclint` — missing or malformed doc comments
- `errorlint` — improper error wrapping/comparison
- `nestif` — deeply nested conditionals
- `gocognit` / `gocyclo` — high complexity functions
- `ireturn` — returning concrete types instead of interfaces
- `gosec` — security issues

---

## Checklist: Security (WebSocket only)

- [ ] No `InsecureSkipVerify: true` in production handlers
- [ ] `*websocket.AcceptOptions` injected as parameter, not hardcoded
- [ ] `OriginPatterns` set for cross-origin restrictions

---

## Final gate

Before approving:

```bash
golangci-lint run ./...    # must be 0 issues
go test -race ./...        # must be green
govulncheck ./...          # must be clean
```

All three must pass. If any fails, fix and re-review.
