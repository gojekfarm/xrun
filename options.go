package xrun

import (
	"time"
)

const (
	// NoTimeout waits indefinitely and never times out
	NoTimeout = time.Duration(0)
)

// Option changes behaviour of Manager
type Option func(*manager)

// WithGracefulShutdownTimeout allows max timeout after which Manager exits
func WithGracefulShutdownTimeout(timeout time.Duration) Option {
	return func(m *manager) {
		m.shutdownTimeout = timeout
	}
}
