// Package fx runs xrun components under an Fx application lifecycle.
package fx

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/gojekfarm/xrun"
	uberfx "go.uber.org/fx"
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
}

// New registers one lifecycle hook pair for components. Components must be
// non-nil. An empty list is valid and completes as soon as Fx starts.
//
// OnStart launches Run with a private lifetime context and does not wait for
// readiness. Callers must acquire resources and bind listeners in an earlier
// OnStart hook if startup must fail synchronously. Register cleanup hooks before
// this hook so Fx calls them after Runner's OnStop, and guard cleanup with
// Active: Fx may continue invoking later OnStop hooks after a hook error, and
// may skip Runner's OnStart entirely while rolling back a failed startup.
func New(lc uberfx.Lifecycle, shutdowner uberfx.Shutdowner, components ...xrun.Component) (*Runner, error) {
	if isNil(lc) || isNil(shutdowner) {
		return nil, errors.New("Fx lifecycle and shutdowner are required")
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
	}
	lc.Append(uberfx.Hook{OnStart: r.start, OnStop: r.stop})
	return r, nil
}

// Done closes only after every component's Run call has returned. A stop
// timeout does not close Done or prove that component-owned resources are safe
// to release.
func (r *Runner) Done() <-chan struct{} { return r.done }

// Active reports whether any launched Run call (or its failure notification)
// remains outstanding. It is false before OnStart, including when Fx skips
// that hook while rolling back a failed startup. Cleanup hooks can use Active
// to avoid closing resources still used by components.
func (r *Runner) Active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.remaining > 0
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
	defer r.mu.Unlock()
	if r.started {
		return errors.New("component runner already started")
	}
	r.started = true
	if err := ctx.Err(); err != nil {
		r.stopping = true
		close(r.done)
		return err
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.remaining = len(r.components)
	if r.remaining == 0 {
		close(r.done)
		return nil
	}
	for i, component := range r.components {
		go r.run(i, component)
	}
	return nil
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

	select {
	case <-r.done:
		return r.Err()
	case <-ctx.Done():
		// Prefer a completed join if it raced with the deadline.
		select {
		case <-r.done:
			return r.Err()
		default:
		}
		return errors.Join(r.Err(), fmt.Errorf("components still running after Fx stop deadline: %w", ctx.Err()))
	}
}
