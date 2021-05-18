package xrun

import (
	"context"
)

// Component allows a component to be started.
// It's very important that Start blocks until
// it's done running.
type Component interface {
	// Start starts running the component. The component will stop running
	// when the context is closed. Start blocks until the context is closed or
	// an error occurs.
	Start(context.Context) error
}

// ComponentFunc is an helper to implement Component inline.
// The component will stop running when the context is closed.
// ComponentFunc must block until the context is closed or an error occurs.
type ComponentFunc func(ctx context.Context) error

// Start starts running the component. The component will stop running
// when the context is closed. Start blocks until the context is closed or
// an error occurs.
func (f ComponentFunc) Start(ctx context.Context) error {
	return f(ctx)
}
