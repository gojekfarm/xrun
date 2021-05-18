package xrun

import (
	"context"
	"time"
)

// All is a utility function which creates a new Manager
// and adds all the components to it. Calling .Start()
// on returned ComponentFunc will call Start on the Manager
func All(shutdownTimeout time.Duration, components ...Component) ComponentFunc {
	m := NewManager(WithGracefulShutdownTimeout(shutdownTimeout))
	for _, c := range components {
		// we can ignore error as `m` is not returned
		// and no one can call m.Add() outside
		_ = m.Add(c)
	}
	return func(ctx context.Context) error {
		return m.Start(ctx)
	}
}
