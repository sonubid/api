package queue

import "errors"

// ErrQueueFull is returned by Enqueue when the internal buffer is at capacity
// and the event cannot be accepted without blocking.
var ErrQueueFull = errors.New("queue is full")
