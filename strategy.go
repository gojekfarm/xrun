package xrun

import "time"

// Strategy defines the order of starting and stopping components
type Strategy int

const (
	// DefaultStartStop starts and stops components in any order
	DefaultStartStop Strategy = iota
	// OrderedStart starts components in order they were added and stops them in random order
	OrderedStart
)

// MaxStartWait allows to set max wait time for component to start when using OrderedStart strategy
type MaxStartWait time.Duration

const defaultMaxStartWait = 5 * time.Minute

func (s Strategy) apply(m *Manager)     { m.strategy = s }
func (t MaxStartWait) apply(m *Manager) { m.maxStartWait = time.Duration(t) }
