package xrun

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// NewManager creates a Manager and applies provided Option
func NewManager(opts ...Option) *Manager {
	m := &Manager{shutdownTimeout: NoTimeout}

	for _, o := range opts {
		o.apply(m)
	}

	return m
}

// Manager helps to run multiple components
// and waits for them to complete
type Manager struct {
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
func (m *Manager) Add(c Component) error {
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
func (m *Manager) Run(ctx context.Context) (err error) {
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

func (m *Manager) start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true

	for _, c := range m.components {
		if c != nil {
			m.startComponent(c)
		}
	}
}

func (m *Manager) startComponent(c Component) {
	m.wg.Add(1)

	go func() {
		defer m.wg.Done()

		if err := c.Run(m.internalCtx); err != nil && !errors.Is(err, context.Canceled) {
			m.errChan <- err
		}
	}()
}

func (m *Manager) engageStopProcedure() error {
	shutdownCancel := m.cancelFunc()
	defer shutdownCancel()

	m.internalCancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopping = true

	var retErr error

	retErrCh := make(chan error, 1)

	go m.aggregateErrors(retErrCh)
	go func() {
		m.wg.Wait()
		close(m.errChan)

		retErr = <-retErrCh

		shutdownCancel()
	}()

	<-m.shutdownCtx.Done()

	if err := m.shutdownCtx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("not all components were shutdown completely within grace period(%s): %w", m.shutdownTimeout, err)
	}

	return retErr
}

func (m *Manager) cancelFunc() context.CancelFunc {
	var shutdownCancel context.CancelFunc
	if m.shutdownTimeout > 0 {
		m.shutdownCtx, shutdownCancel = context.WithTimeout(context.Background(), m.shutdownTimeout)
	} else {
		m.shutdownCtx, shutdownCancel = context.WithCancel(context.Background())
	}

	return shutdownCancel
}

func (m *Manager) aggregateErrors(ch chan<- error) {
	var r error
	for err := range m.errChan {
		r = errors.Join(r, err)
	}
	ch <- r
}
