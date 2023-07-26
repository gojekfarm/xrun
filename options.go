package xrun

import (
	"time"
)

const (
	// NoTimeout waits indefinitely and never times out
	NoTimeout = time.Duration(0)
)

// Option changes behaviour of Manager
type Option interface {
	apply(*Manager)
}

// ShutdownTimeout allows max timeout after which Manager exits.
type ShutdownTimeout time.Duration

func (t ShutdownTimeout) apply(m *Manager) { m.shutdownTimeout = time.Duration(t) }
