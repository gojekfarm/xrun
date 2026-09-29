package fx_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/gojekfarm/xrun"
	xfx "github.com/gojekfarm/xrun/component/x/fx"
)

func appWithComponents(t *testing.T, components ...xrun.Component) (*fx.App, *xfx.Runner) {
	t.Helper()
	var runner *xfx.Runner
	app := fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle, shutdowner fx.Shutdowner) error {
		var err error
		runner, err = xfx.New(lc, shutdowner, components...)
		return err
	}))
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = app.Stop(ctx)
	})
	return app, runner
}

func start(t *testing.T, app *fx.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
}

func stop(t *testing.T, app *fx.App) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return app.Stop(ctx)
}

func wait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for component")
	}
}

func TestStartContextIsNotLifetimeContext(t *testing.T) {
	received := make(chan context.Context, 1)
	finished := make(chan struct{})
	app, runner := appWithComponents(t, xrun.ComponentFunc(func(ctx context.Context) error {
		received <- ctx
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}))
	startup, cancel := context.WithCancel(context.Background())
	if err := app.Start(startup); err != nil {
		t.Fatal(err)
	}
	var lifetime context.Context
	select {
	case lifetime = <-received:
	case <-time.After(time.Second):
		t.Fatal("component did not start")
	}
	cancel()
	if err := lifetime.Err(); err != nil {
		t.Fatalf("startup cancellation stopped the component: %v", err)
	}
	if err := stop(t, app); err != nil {
		t.Fatal(err)
	}
	wait(t, finished)
	wait(t, runner.Done())
	if err := runner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestFailureRequestsNonzeroShutdownAndPreservesError(t *testing.T) {
	failure := errors.New("serve failed")
	ready := make(chan struct{})
	release := make(chan struct{})
	app, runner := appWithComponents(t,
		xrun.ComponentFunc(func(ctx context.Context) error { close(ready); <-release; return failure }),
		xrun.ComponentFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }),
	)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	start(t, app)
	wait(t, ready)
	close(release)
	select {
	case signal := <-app.Wait():
		if signal.ExitCode != 1 {
			t.Fatalf("exit code = %d", signal.ExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("Fx shutdown was not requested")
	}
	if err := stop(t, app); !errors.Is(err, failure) {
		t.Fatalf("Stop error = %v", err)
	}
	wait(t, runner.Done())
	if !errors.Is(runner.Err(), failure) {
		t.Fatalf("Runner error = %v", runner.Err())
	}
}

func TestUnexpectedCleanExit(t *testing.T) {
	release := make(chan struct{})
	app, runner := appWithComponents(t, xrun.ComponentFunc(func(context.Context) error { <-release; return nil }))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	start(t, app)
	close(release)
	select {
	case signal := <-app.Wait():
		if signal.ExitCode != 1 {
			t.Fatalf("exit code = %d", signal.ExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("Fx shutdown was not requested")
	}
	if err := stop(t, app); !errors.Is(err, xfx.ErrUnexpectedExit) {
		t.Fatalf("Stop error = %v", err)
	}
	wait(t, runner.Done())
}

func TestStopTimeoutDoesNotClaimJoin(t *testing.T) {
	running := make(chan struct{})
	release := make(chan struct{})
	app, runner := appWithComponents(t, xrun.ComponentFunc(func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		<-release
		return nil
	}))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	start(t, app)
	wait(t, running)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := app.Stop(ctx)
	// Fx can return its own deadline error before the hook's error.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v", err)
	}
	select {
	case <-runner.Done():
		t.Fatal("Done closed before Run exited")
	default:
	}
	close(release)
	wait(t, runner.Done())
	if err := runner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestStopHookReportsUnjoinedComponent(t *testing.T) {
	lc := &hookCapture{}
	running := make(chan struct{})
	release := make(chan struct{})
	runner, err := xfx.New(lc, noShutdown{}, xrun.ComponentFunc(func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		<-release
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	wait(t, running)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err = lc.hook.OnStop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "components still running") {
		t.Fatalf("OnStop error = %v", err)
	}
	close(release)
	wait(t, runner.Done())
}

func TestConcurrentFailuresPreserved(t *testing.T) {
	first := errors.New("first failure")
	second := errors.New("second failure")
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(2)
	app, runner := appWithComponents(t,
		xrun.ComponentFunc(func(context.Context) error { started.Done(); <-release; return first }),
		xrun.ComponentFunc(func(context.Context) error { started.Done(); <-release; return second }),
	)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	start(t, app)
	started.Wait()
	close(release)
	wait(t, runner.Done())
	err := stop(t, app)
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("Stop error = %v", err)
	}
}

func TestJoinedCancellationAndFailurePreserved(t *testing.T) {
	failure := errors.New("failed during cancellation")
	running := make(chan struct{})
	app, runner := appWithComponents(t, xrun.ComponentFunc(func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		return errors.Join(ctx.Err(), failure)
	}))
	start(t, app)
	wait(t, running)
	if err := stop(t, app); !errors.Is(err, failure) {
		t.Fatalf("Stop error = %v", err)
	}
	if !errors.Is(runner.Err(), failure) {
		t.Fatalf("Runner error = %v", runner.Err())
	}
}

func TestInvalidAndEmptyComponents(t *testing.T) {
	app, runner := appWithComponents(t)
	start(t, app)
	wait(t, runner.Done())
	if err := stop(t, app); err != nil {
		t.Fatal(err)
	}
	var nilComponent *nilRunner
	app = fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle, shutdowner fx.Shutdowner) error {
		_, err := xfx.New(lc, shutdowner, nilComponent)
		return err
	}))
	if app.Err() == nil || !strings.Contains(app.Err().Error(), "component 0 is nil") {
		t.Fatalf("expected nil component error, got %v", app.Err())
	}
}

func TestRejectedStartSettlesDone(t *testing.T) {
	lc := &hookCapture{}
	runner, err := xfx.New(lc, noShutdown{}, xrun.ComponentFunc(func(context.Context) error {
		t.Fatal("component should not run")
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lc.hook.OnStart(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("OnStart error = %v", err)
	}
	wait(t, runner.Done())
	if runner.Active() {
		t.Fatal("runner active after rejected start")
	}
}

func TestFxStartupRollbackCanSkipRunnerHook(t *testing.T) {
	cleaned := make(chan struct{})
	var runner *xfx.Runner
	app := fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle, shutdowner fx.Shutdowner) error {
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error { return nil },
			OnStop: func(context.Context) error {
				if runner.Active() {
					return errors.New("resource still in use")
				}
				close(cleaned)
				return nil
			},
		})
		lc.Append(fx.Hook{OnStart: func(context.Context) error { return errors.New("startup failed") }})
		var err error
		runner, err = xfx.New(lc, shutdowner, xrun.ComponentFunc(func(context.Context) error {
			t.Fatal("component should not run")
			return nil
		}))
		return err
	}))
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	if err := app.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "startup failed") {
		t.Fatalf("Start error = %v", err)
	}
	wait(t, cleaned)
	if runner.Active() {
		t.Fatal("runner active after startup rollback")
	}
}

type nilRunner struct{}

func (*nilRunner) Run(context.Context) error { return nil }

type hookCapture struct{ hook fx.Hook }

func (h *hookCapture) Append(hook fx.Hook) { h.hook = hook }

type noShutdown struct{}

func (noShutdown) Shutdown(...fx.ShutdownOption) error { return nil }
