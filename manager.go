package xrun

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Manager helps to run multiple components
// and waits for them to complete
type Manager interface {
	Component

	// Add will include the Component, and the Component will
	// start running when Run is called.
	Add(Component) error
}

// NewManager creates a Manager and applies provided Option
func NewManager(opts ...Option) Manager {
	m := &manager{
		shutdownTimeout: NoTimeout,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

type manager struct {
	mu sync.Mutex

	internalCtx    context.Context
	internalCancel context.CancelFunc

	components []Component
	wg         sync.WaitGroup

	started         bool
	stopping        bool
	shutdownTimeout time.Duration
	shutdownCtx     context.Context
	errChan         chan error
}

// Add will enqueue the Component to run it,
// last added component will be started first
func (m *manager) Add(c Component) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopping {
		return errors.New("can't accept new component as stop procedure is already engaged")
	}

	if m.started {
		return errors.New("can't accept new component as manager has already started")
	}

	m.components = append(m.components, c)
	return nil
}

// Run starts running the registered components. The components will stop running
// when the context is closed. Run blocks until the context is closed or
// an error occurs.
func (m *manager) Run(ctx context.Context) (err error) {
	m.internalCtx, m.internalCancel = context.WithCancel(ctx)

	defer func() {
		if stopErr := m.engageStopProcedure(); stopErr != nil {
			err = stopErr
		}
	}()

	m.errChan = make(chan error)

	go m.start()

	select {
	case <-ctx.Done():
		return
	case err := <-m.errChan:
		return err
	}
}

func (m *manager) start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true

	for _, c := range m.components {
		if c != nil {
			m.startComponent(c)
		}
	}
}

func (m *manager) startComponent(c Component) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := c.Run(m.internalCtx); err != nil {
			m.errChan <- err
		}
	}()
}

func (m *manager) engageStopProcedure() error {
	var shutdownCancel context.CancelFunc
	if m.shutdownTimeout > 0 {
		m.shutdownCtx, shutdownCancel = context.WithTimeout(context.Background(), m.shutdownTimeout)
	} else {
		m.shutdownCtx, shutdownCancel = context.WithCancel(context.Background())
	}
	defer shutdownCancel()

	m.internalCancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopping = true

	go func() {
		m.wg.Wait()
		shutdownCancel()
	}()

	<-m.shutdownCtx.Done()
	if err := m.shutdownCtx.Err(); err != nil && err != context.Canceled {
		return fmt.Errorf("not all components were shutdown completely within grace period(%s): %w", m.shutdownTimeout, err)
	}
	return nil
}
