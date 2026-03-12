package server

import "errors"

// ErrInvalidAddress is returned by Start when the provided addr is empty.
var ErrInvalidAddress = errors.New("server: invalid address")

// ErrNilLogger is returned by Start when a nil logger is provided.
var ErrNilLogger = errors.New("server: nil logger")

// ErrNilHandler is returned by Start when a nil handler is provided.
var ErrNilHandler = errors.New("server: nil handler")
