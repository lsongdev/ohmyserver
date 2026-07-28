// Package terminal connects interactive sessions to transports such as WebSocket.
// A session may be backed by a local process, SSH, a container, or another PTY.
package terminal

import (
	"context"
	"io"
)

// Session is the smallest useful interactive terminal abstraction.
type Session interface {
	Input() io.WriteCloser
	Output() io.Reader
	Resize(cols, rows uint16) error
	Wait() error
	Close() error
}

// Backend opens a terminal session. Implementations own connection and process
// details; callers only need to bridge the returned session.
type Backend interface {
	Open(ctx context.Context, cols, rows uint16) (Session, error)
}

// BackendFunc adapts a function to Backend.
type BackendFunc func(ctx context.Context, cols, rows uint16) (Session, error)

func (f BackendFunc) Open(ctx context.Context, cols, rows uint16) (Session, error) {
	return f(ctx, cols, rows)
}
