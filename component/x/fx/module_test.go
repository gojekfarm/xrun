package fx_test

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/gojekfarm/xrun"
	xfx "github.com/gojekfarm/xrun/component/x/fx"
)

type namedAddresses struct {
	fx.In
	Public string `name:"public"`
	Admin  string `name:"admin"`
}

type namedServers struct {
	fx.Out
	Public     *testServer      `name:"public"`
	Admin      *testServer      `name:"admin"`
	Components []xrun.Component `group:"xrun,flatten"`
}

func provideNamedServers(in namedAddresses) namedServers {
	public := &testServer{address: in.Public}
	admin := &testServer{address: in.Admin}
	return namedServers{
		Public: public, Admin: admin,
		Components: []xrun.Component{public, admin},
	}
}

type namedInputs struct {
	fx.In
	Public *testServer `name:"public"`
	Admin  *testServer `name:"admin"`
}

type testServer struct {
	address  string
	listener net.Listener
	prepared atomic.Int32
	runs     atomic.Int32
	closes   atomic.Int32
}

func (s *testServer) Prepare(context.Context) error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	s.listener = listener
	s.prepared.Add(1)

	return nil
}

func (s *testServer) Run(ctx context.Context) error {
	s.runs.Add(1)
	<-ctx.Done()

	return nil
}

func (s *testServer) Close(context.Context) error {
	s.closes.Add(1)

	return s.listener.Close()
}

func TestModuleRunsTwoNamedInstances(t *testing.T) {
	var injected namedInputs
	app := fx.New(
		fx.NopLogger,
		fx.Provide(
			fx.Annotate(func() string { return "127.0.0.1:0" }, fx.ResultTags(`name:"public"`)),
			fx.Annotate(func() string { return "127.0.0.1:0" }, fx.ResultTags(`name:"admin"`)),
			provideNamedServers,
		),
		xfx.Module(),
		fx.Invoke(func(in namedInputs) { injected = in }),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}

	start(t, app)
	if injected.Public == nil || injected.Admin == nil || injected.Public == injected.Admin {
		t.Fatal("named injections were not distinct instances")
	}

	for _, server := range []*testServer{injected.Public, injected.Admin} {
		if server.prepared.Load() != 1 || server.listener == nil {
			t.Fatal("listener was not prepared at startup")
		}
	}

	if err := stop(t, app); err != nil {
		t.Fatal(err)
	}

	for _, server := range []*testServer{injected.Public, injected.Admin} {
		if server.runs.Load() != 1 || server.closes.Load() != 1 {
			t.Fatalf("run/close counts = %d/%d", server.runs.Load(), server.closes.Load())
		}
	}

	if err := stop(t, app); err != nil {
		t.Fatal(err)
	}

	for _, server := range []*testServer{injected.Public, injected.Admin} {
		if server.closes.Load() != 1 {
			t.Fatal("Close called again")
		}
	}
}

func TestModuleBindFailureAbortsStartup(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()

	var injected namedInputs
	app := fx.New(
		fx.NopLogger,
		fx.Provide(
			fx.Annotate(func() string { return "127.0.0.1:0" }, fx.ResultTags(`name:"public"`)),
			fx.Annotate(func() string { return occupied.Addr().String() }, fx.ResultTags(`name:"admin"`)),
			provideNamedServers,
		),
		xfx.Module(),
		fx.Invoke(func(in namedInputs) { injected = in }),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.Start(ctx); err == nil {
		t.Fatal("expected bind failure")
	}

	for _, server := range []*testServer{injected.Public, injected.Admin} {
		if server.runs.Load() != 0 {
			t.Fatal("Run started despite failed Prepare")
		}
		if server.closes.Load() != server.prepared.Load() {
			t.Fatalf("prepared/close counts = %d/%d", server.prepared.Load(), server.closes.Load())
		}
	}
}

func TestDuplicateModuleRejected(t *testing.T) {
	app := fx.New(fx.NopLogger, xfx.Module(), xfx.Module())
	if app.Err() == nil {
		t.Fatal("duplicate module registration accepted")
	}
}

type preparedStub struct {
	prepare func(context.Context) error
	run     func(context.Context) error
	close   func(context.Context) error
}

func (s preparedStub) Prepare(ctx context.Context) error { return s.prepare(ctx) }
func (s preparedStub) Run(ctx context.Context) error     { return s.run(ctx) }
func (s preparedStub) Close(ctx context.Context) error   { return s.close(ctx) }

func TestPreparedRollbackAndReverseClose(t *testing.T) {
	lc := &hookCapture{}
	prepareErr := errors.New("prepare failure")
	var closes []int
	component := func(index int) preparedStub {
		return preparedStub{
			prepare: func(context.Context) error { return nil },
			run:     func(context.Context) error { t.Fatal("Run should not start"); return nil },
			close: func(context.Context) error {
				closes = append(closes, index)
				return nil
			},
		}
	}
	failed := preparedStub{
		prepare: func(context.Context) error { return prepareErr },
		run:     func(context.Context) error { t.Fatal("Run should not start"); return nil },
		close:   func(context.Context) error { t.Fatal("failed Prepare must own its rollback"); return nil },
	}
	runner, err := xfx.New(lc, noShutdown{}, component(0), component(1), failed)
	if err != nil {
		t.Fatal(err)
	}

	if err := lc.hook.OnStart(context.Background()); !errors.Is(err, prepareErr) {
		t.Fatalf("Start error = %v", err)
	}
	wait(t, runner.Done())
	if runner.Active() || len(closes) != 2 || closes[0] != 1 || closes[1] != 0 {
		t.Fatalf("rollback order = %v, active = %v", closes, runner.Active())
	}
	if err := lc.hook.OnStop(context.Background()); !errors.Is(err, prepareErr) {
		t.Fatalf("Stop error = %v", err)
	}
	if len(closes) != 2 {
		t.Fatal("rollback Close called again")
	}
}
