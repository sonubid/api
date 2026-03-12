---
name: websocket
description: WebSocket usage rules for github.com/coder/websocket including connection lifecycle, security, and context propagation
license: MIT
compatibility: opencode
---

## When to use this skill

Load this skill whenever you are working in `internal/hub` or any code that touches WebSocket connections, handlers, or clients.

---

## Library

Use `github.com/coder/websocket` (formerly `nhooyr.io/websocket`). Do not use other WebSocket libraries.

---

## Connection lifecycle

### Closing connections

- `conn.CloseNow()` — immediate, unilateral close. Use in **cleanup paths and defers**.
- `conn.Close()` — performs the full closing handshake. Use when the **local side initiates** a clean close.

Never use `Close()` in a defer; use `CloseNow()` there instead.

### Dialing

`websocket.Dial()` returns `(*Conn, *http.Response, error)`. The response body must always be closed:

```go
conn, resp, err := websocket.Dial(ctx, url, nil)
if resp != nil && resp.Body != nil {
    resp.Body.Close()
}
if err != nil {
    return err
}
```

---

## Security

- Never ship `InsecureSkipVerify: true` in production handlers.
- Inject `*websocket.AcceptOptions` as a parameter to handlers instead of hardcoding options.
- Pass `opts.OriginPatterns` to `websocket.Accept` to restrict cross-origin connections in production.

```go
// Correct: options injected as parameter
func NewHandler(hub *Hub, opts *websocket.AcceptOptions) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        conn, err := websocket.Accept(w, r, opts)
        // ...
    }
}
```

---

## Context propagation

Handlers must derive context from `r.Context()` so that server shutdown propagates to all active WebSocket sessions:

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    conn, err := websocket.Accept(w, r, h.opts)
    if err != nil {
        return
    }
    defer conn.CloseNow()

    ctx := r.Context() // NOT context.Background()
    h.hub.Register(ctx, conn)
}
```

Never use `context.Background()` in production WebSocket paths.
