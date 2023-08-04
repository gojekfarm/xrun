package xrun

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
)

// NewManager creates a Manager and applies provided Option
func NewManager(opts ...Option) *Manager {
	m := &Manager{
		shutdownTimeout: NoTimeout,
		maxStartWait:    defaultMaxStartWait,
	}

	for _, o := range opts {
		o.apply(m)
	}

	return m
}

// Manager helps to run multiple components
// and waits for them to complete
type Manager struct {
	strategy     Strategy
	maxStartWait time.Duration
	mu           sync.Mutex

	internalCtx    context.Context
	internalCancel context.CancelFunc

	components       []Component
	componentCancels []context.CancelFunc
	wg               sync.WaitGroup

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

	switch m.strategy {
	case OrderedStart:
		for _, c := range m.components {
			if c != nil {
				notify := newNotifyCtx(m.internalCtx)
				nCtx, cancel := context.WithCancel(notify)
				m.startComponent(c, nCtx)
				m.componentCancels = append([]context.CancelFunc{cancel}, m.componentCancels...)

				// Block until the component has started or the timeout has elapsed.
				select {
				case <-started(notify):
				case <-time.After(m.maxStartWait):
				}
			}
		}
	case DefaultStartStop:
		for _, c := range m.components {
			if c != nil {
				m.startComponent(c, m.internalCtx)
			}
		}
	}
}

func (m *Manager) startComponent(c Component, ctx context.Context) {
	m.wg.Add(1)

	go func() {
		defer m.wg.Done()

		if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			m.errChan <- err
		}
	}()
}

func (m *Manager) engageStopProcedure() error {
	shutdownCancel := m.cancelFunc()
	defer shutdownCancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopping = true

	switch m.strategy {
	case OrderedStart:
		for _, cancel := range m.componentCancels {
			cancel()
		}
	case DefaultStartStop:
		m.internalCancel()
	}

	var retErr *multierror.Error

	retErrCh := make(chan *multierror.Error, 1)

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

	return retErr.ErrorOrNil()
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

func (m *Manager) aggregateErrors(ch chan<- *multierror.Error) {
	var r *multierror.Error
	for err := range m.errChan {
		r = multierror.Append(r, err)
	}
	ch <- r
}
