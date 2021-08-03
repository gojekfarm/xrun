package xrun

import (
	"context"
)

// Component allows a component to be started.
// It's very important that Run blocks until
// it's done running.
type Component interface {
	// Run starts running the component. The component will stop running
	// when the context is closed. Run blocks until the context is closed or
	// an error occurs.
	Run(context.Context) error
}

// ComponentFunc is a helper to implement Component inline.
// The component will stop running when the context is closed.
// ComponentFunc must block until the context is closed or an error occurs.
type ComponentFunc func(ctx context.Context) error

// Run starts running the component. The component will stop running
// when the context is closed. Run blocks until the context is closed or
// an error occurs.
func (f ComponentFunc) Run(ctx context.Context) error {
	return f(ctx)
}
