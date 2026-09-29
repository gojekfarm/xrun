// Package fx runs xrun components under an Fx application lifecycle.
package fx

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	uberfx "go.uber.org/fx"

	"github.com/gojekfarm/xrun"
)

// ErrUnexpectedExit means a long-running component returned nil before its
// lifetime context was canceled.
var ErrUnexpectedExit = errors.New("component exited before shutdown")

// Runner supervises a fixed set of components. Err and Done remain usable
// after Fx stops, including when an OnStop deadline expires before Run returns.
type Runner struct {
	components []xrun.Component
	shutdowner uberfx.Shutdowner

	mu        sync.Mutex
	started   bool
	stopping  bool
	cancel    context.CancelFunc
	ctx       context.Context
	remaining int
	done      chan struct{}
	err       error
	prepared  []preparedEntry
	closing   bool
	closed    chan struct{}
}

type preparedEntry struct {
	index     int
	component PreparedComponent
}

// New registers one lifecycle hook pair for components. Components must be
// non-nil. An empty list is valid and completes as soon as Fx starts.
//
// OnStart calls Prepare on each PreparedComponent before launching any Run.
// Other components have no readiness handshake. Callers can own resources in
// an earlier OnStart hook, registering its cleanup before this hook so Fx calls
// it after Runner's OnStop. Such cleanup must check Active: Fx may continue
// after a hook error or skip Runner's hook during startup rollback.
func New(lc uberfx.Lifecycle, shutdowner uberfx.Shutdowner, components ...xrun.Component) (*Runner, error) {
	if isNil(lc) || isNil(shutdowner) {
		return nil, errors.New("fx lifecycle and shutdowner are required")
	}

	for i, component := range components {
		if isNil(component) {
			return nil, fmt.Errorf("component %d is nil", i)
		}
	}

	r := &Runner{
		components: append([]xrun.Component(nil), components...),
		shutdowner: shutdowner,
		done:       make(chan struct{}),
		closed:     make(chan struct{}),
	}
	lc.Append(uberfx.Hook{OnStart: r.start, OnStop: r.stop})

	return r, nil
}

// Done closes only after every component's Run call has returned. A stop
// timeout does not close Done or prove that component-owned resources are safe
// to release.
func (r *Runner) Done() <-chan struct{} { return r.done }

// Active reports whether preparation, a launched Run, failure notification, or
// component cleanup remains outstanding. It is false before OnStart, including
// when Fx skips that hook during rollback. Other cleanup hooks can use Active
// to avoid closing resources still used by components.
func (r *Runner) Active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.remaining > 0 || r.closing
}

// Err returns all observed unexpected Run and shutdown notification errors.
// A normal requested cancellation does not add an error.
func (r *Runner) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.err
}

func (r *Runner) start(ctx context.Context) error {
	r.mu.Lock()

	if r.started {
		r.mu.Unlock()

		return errors.New("component runner already started")
	}

	r.started = true
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.remaining = 1 // Startup preparation is active until committed or rolled back.
	r.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return r.failStart(err, nil, ctx)
	}

	var prepared []preparedEntry

	for i, component := range r.components {
		if p, ok := component.(PreparedComponent); ok {
			if err := p.Prepare(ctx); err != nil {
				return r.failStart(fmt.Errorf("prepare component %d: %w", i, err), prepared, ctx)
			}

			prepared = append(prepared, preparedEntry{index: i, component: p})
		}

		if err := ctx.Err(); err != nil {
			return r.failStart(err, prepared, ctx)
		}
	}

	r.mu.Lock()
	startErr := ctx.Err()

	if startErr == nil && (r.stopping || r.ctx.Err() != nil) {
		startErr = context.Canceled
	}

	if startErr != nil {
		r.mu.Unlock()

		return r.failStart(startErr, prepared, ctx)
	}

	r.prepared = prepared
	r.remaining = len(r.components)

	if r.remaining == 0 {
		close(r.done)
		r.mu.Unlock()

		return nil
	}

	for i, component := range r.components {
		go r.run(i, component)
	}
	r.mu.Unlock()

	return nil
}

func (r *Runner) failStart(startErr error, prepared []preparedEntry, ctx context.Context) error {
	r.mu.Lock()
	r.stopping = true
	r.cancel()
	r.closing = true
	r.mu.Unlock()

	closeErr := closePrepared(ctx, prepared)
	result := errors.Join(startErr, closeErr)

	r.mu.Lock()
	r.err = errors.Join(r.err, result)
	r.closing = false
	r.remaining = 0
	close(r.closed)
	close(r.done)
	r.mu.Unlock()

	return result
}

func closePrepared(ctx context.Context, prepared []preparedEntry) error {
	var result error

	for i := len(prepared) - 1; i >= 0; i-- {
		entry := prepared[i]
		if err := entry.component.Close(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("close component %d: %w", entry.index, err))
		}
	}

	return result
}

func (r *Runner) run(index int, component xrun.Component) {
	err := component.Run(r.ctx)

	r.mu.Lock()
	notify := false
	unexpected := r.ctx.Err() == nil

	if err == nil && unexpected {
		err = ErrUnexpectedExit
	}

	if err != nil && (!onlyCancellation(err) || unexpected) {
		r.err = errors.Join(r.err, fmt.Errorf("component %d: %w", index, err))

		if !r.stopping {
			r.stopping = true
			r.cancel()

			notify = true
		}
	}
	r.mu.Unlock()

	if notify {
		// This component goroutine owns the notification. Done is closed
		// only after the notification finishes.
		r.notifyShutdown()
	}

	r.mu.Lock()
	r.remaining--

	if r.remaining == 0 {
		close(r.done)
	}
	r.mu.Unlock()
}

func isNil(value interface{}) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// onlyCancellation accepts wrapped or joined cancellation errors while keeping
// any other joined failure visible, even if cancellation happened concurrently.
func onlyCancellation(err error) bool {
	if err == context.Canceled {
		return true
	}

	if many, ok := err.(interface{ Unwrap() []error }); ok {
		children := many.Unwrap()

		if len(children) == 0 {
			return false
		}

		for _, child := range children {
			if !onlyCancellation(child) {
				return false
			}
		}

		return true
	}

	if one, ok := err.(interface{ Unwrap() error }); ok {
		return onlyCancellation(one.Unwrap())
	}

	return false
}

func (r *Runner) notifyShutdown() {
	if err := r.shutdowner.Shutdown(uberfx.ExitCode(1)); err != nil {
		r.mu.Lock()
		r.err = errors.Join(r.err, fmt.Errorf("request Fx shutdown: %w", err))
		r.mu.Unlock()
	}
}

func (r *Runner) stop(ctx context.Context) error {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()

		return nil
	}

	r.stopping = true

	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()

	if ctx.Err() != nil {
		return r.stopDeadline(ctx)
	}

	select {
	case <-r.done:
		return r.closeAfterJoin(ctx)
	case <-ctx.Done():
		return r.stopDeadline(ctx)
	}
}

func (r *Runner) stopDeadline(ctx context.Context) error {
	// Prefer a completed join if it raced with the deadline.
	select {
	case <-r.done:
		return r.closeAfterJoin(ctx)
	default:
	}

	return errors.Join(r.Err(), fmt.Errorf("components still running after Fx stop deadline: %w", ctx.Err()))
}

func (r *Runner) closeAfterJoin(ctx context.Context) error {
	r.mu.Lock()
	if r.closing {
		closed := r.closed
		r.mu.Unlock()

		select {
		case <-closed:
			return r.Err()
		case <-ctx.Done():
			return errors.Join(r.Err(), fmt.Errorf("component cleanup still running after Fx stop deadline: %w", ctx.Err()))
		}
	}

	select {
	case <-r.closed:
		r.mu.Unlock()

		return r.Err()
	default:
	}

	r.closing = true
	prepared := r.prepared
	r.mu.Unlock()

	closeErr := closePrepared(ctx, prepared)

	r.mu.Lock()
	r.err = errors.Join(r.err, closeErr)
	r.closing = false
	close(r.closed)
	r.mu.Unlock()

	return r.Err()
}
