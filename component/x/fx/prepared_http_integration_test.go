package fx_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gojekfarm/xrun/component"
	xfx "github.com/gojekfarm/xrun/component/x/fx"
)

func TestPreparedHTTPFatalExitKeepsOtherOwnersAliveUntilHandlerReturns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	handlerReturned := make(chan struct{})
	ownerClosed := make(chan struct{})
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release // Deliberately ignores cancellation.
		close(handlerReturned)
		w.WriteHeader(http.StatusNoContent)
	})}
	httpComponent, err := component.NewPreparedHTTPServer(server, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	owner := preparedStub{
		prepare: func(context.Context) error { return nil },
		run:     func(ctx context.Context) error { <-ctx.Done(); return nil },
		close: func(context.Context) error {
			select {
			case <-handlerReturned:
				close(ownerClosed)
				return nil
			default:
				return errors.New("owner closed while HTTP handler was active")
			}
		},
	}
	lc := &hookCapture{}
	runner, err := xfx.New(lc, noShutdown{}, owner, httpComponent, httpComponent.HandlerDrain())
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan struct{})
	go func() {
		resp, err := http.Get("http://" + httpComponent.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	wait(t, started)
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := lc.hook.OnStop(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop with active handler = %v", err)
	}
	select {
	case <-ownerClosed:
		t.Fatal("owner closed before handler returned")
	default:
	}
	close(release)
	wait(t, handlerReturned)
	wait(t, runner.Done())
	if err := lc.hook.OnStop(context.Background()); err == nil {
		t.Fatal("fatal HTTP error was lost")
	}
	wait(t, ownerClosed)
	wait(t, requestDone)
}
