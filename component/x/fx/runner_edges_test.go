package fx_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/gojekfarm/xrun"
	xfx "github.com/gojekfarm/xrun/component/x/fx"
)

func TestInvalidDependenciesAndRepeatedStart(t *testing.T) {
	if _, err := xfx.New(nil, noShutdown{}); err == nil {
		t.Fatal("nil lifecycle accepted")
	}
	if _, err := xfx.New(&hookCapture{}, nil); err == nil {
		t.Fatal("nil shutdowner accepted")
	}
	if _, err := xfx.New(&hookCapture{}, noShutdown{}, nil); err == nil {
		t.Fatal("nil component accepted")
	}

	lc := &hookCapture{}
	runner, err := xfx.New(lc, noShutdown{})
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err == nil {
		t.Fatal("second start accepted")
	}
	if err := lc.hook.OnStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lc.hook.OnStop(expired); err != nil {
		t.Fatalf("completed runner with expired stop context: %v", err)
	}
	if runner.Active() {
		t.Fatal("empty runner still active")
	}
}

type failingShutdown struct{ err error }

func (s failingShutdown) Shutdown(...fx.ShutdownOption) error { return s.err }

func TestShutdownNotificationFailureRetained(t *testing.T) {
	lc := &hookCapture{}
	failure := errors.New("run failed")
	notification := errors.New("shutdown failed")
	runner, err := xfx.New(lc, failingShutdown{notification}, xrun.ComponentFunc(func(context.Context) error { return failure }))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	wait(t, runner.Done())
	if err := lc.hook.OnStop(context.Background()); !errors.Is(err, failure) || !errors.Is(err, notification) {
		t.Fatalf("Stop error = %v", err)
	}
}

func TestPrepareCancellationAndCloseFailure(t *testing.T) {
	lc := &hookCapture{}
	startup, cancel := context.WithCancel(context.Background())
	closeFailure := errors.New("close failed")
	prepared := preparedStub{
		prepare: func(context.Context) error { cancel(); return nil },
		run:     func(context.Context) error { t.Fatal("Run should not start"); return nil },
		close:   func(context.Context) error { return closeFailure },
	}
	runner, err := xfx.New(lc, noShutdown{}, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(startup); !errors.Is(err, context.Canceled) || !errors.Is(err, closeFailure) {
		t.Fatalf("Start error = %v", err)
	}
	wait(t, runner.Done())
	if !errors.Is(runner.Err(), closeFailure) {
		t.Fatalf("Runner error = %v", runner.Err())
	}
}

func TestStopDuringPrepareRollsBack(t *testing.T) {
	lc := &hookCapture{}
	preparing := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	prepared := preparedStub{
		prepare: func(context.Context) error { close(preparing); <-release; return nil },
		run:     func(context.Context) error { t.Fatal("Run should not start"); return nil },
		close:   func(context.Context) error { close(closed); return nil },
	}
	runner, err := xfx.New(lc, noShutdown{}, prepared)
	if err != nil {
		t.Fatal(err)
	}
	startErr := make(chan error, 1)
	go func() { startErr <- lc.hook.OnStart(context.Background()) }()
	wait(t, preparing)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := lc.hook.OnStop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v", err)
	}
	close(release)
	if err := <-startErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v", err)
	}
	wait(t, closed)
	wait(t, runner.Done())
	if runner.Active() {
		t.Fatal("runner active after rollback")
	}
}

func TestCloseFailureAndConcurrentStop(t *testing.T) {
	lc := &hookCapture{}
	closing := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	failure := errors.New("close failed")
	prepared := preparedStub{
		prepare: func(context.Context) error { return nil },
		run:     func(ctx context.Context) error { <-ctx.Done(); return nil },
		close:   func(context.Context) error { close(closing); <-release; return failure },
	}
	runner, err := xfx.New(lc, noShutdown{}, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstStop := make(chan error, 1)
	go func() { firstStop <- lc.hook.OnStop(context.Background()) }()
	wait(t, closing)
	if !runner.Active() {
		t.Fatal("runner became inactive during cleanup")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := lc.hook.OnStop(ctx); !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "cleanup still running") {
		t.Fatalf("concurrent Stop error = %v", err)
	}
	joiningStop := make(chan error, 1)
	go func() { joiningStop <- lc.hook.OnStop(context.Background()) }()
	select {
	case err := <-joiningStop:
		t.Fatalf("concurrent Stop returned before cleanup: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-firstStop; !errors.Is(err, failure) {
		t.Fatalf("first Stop error = %v", err)
	}
	if err := <-joiningStop; !errors.Is(err, failure) {
		t.Fatalf("joining Stop error = %v", err)
	}
	if err := lc.hook.OnStop(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("repeated Stop error = %v", err)
	}
}

type emptyJoinedError struct{}

func (emptyJoinedError) Error() string   { return "empty joined error" }
func (emptyJoinedError) Unwrap() []error { return nil }

func TestCancellationClassification(t *testing.T) {
	lc := &hookCapture{}
	running := make(chan struct{})
	runner, err := xfx.New(lc, noShutdown{}, xrun.ComponentFunc(func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		return errors.Join(context.Canceled, errors.Join(context.Canceled, context.Canceled))
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	wait(t, running)
	if err := lc.hook.OnStop(context.Background()); err != nil {
		t.Fatalf("cancellation-only Stop error = %v", err)
	}
	if runner.Err() != nil {
		t.Fatalf("cancellation-only Runner error = %v", runner.Err())
	}

	lc = &hookCapture{}
	running = make(chan struct{})
	_, err = xfx.New(lc, noShutdown{}, xrun.ComponentFunc(func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		return fmt.Errorf("wrapped cancellation: %w", ctx.Err())
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	wait(t, running)
	if err := lc.hook.OnStop(context.Background()); err != nil {
		t.Fatalf("wrapped cancellation Stop error = %v", err)
	}

	lc = &hookCapture{}
	runner, err = xfx.New(lc, noShutdown{}, xrun.ComponentFunc(func(context.Context) error { return emptyJoinedError{} }))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	wait(t, runner.Done())
	if err := runner.Err(); err == nil || !strings.Contains(err.Error(), "empty joined error") {
		t.Fatalf("empty joined error lost: %v", err)
	}
}
