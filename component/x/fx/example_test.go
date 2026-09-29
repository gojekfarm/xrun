package fx_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/gojekfarm/xrun"
	xfx "github.com/gojekfarm/xrun/component/x/fx"
	"go.uber.org/fx"
)

// The listener is bound in an earlier OnStart hook, so a bind failure fails
// startup synchronously. Run owns the accept loop and exits before cleanup.
func ExampleNew() {
	var listener net.Listener
	var runner *xfx.Runner
	app := fx.New(
		fx.NopLogger,
		fx.StopTimeout(20*time.Second),
		fx.Invoke(func(lc fx.Lifecycle, shutdowner fx.Shutdowner) error {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					var err error
					listener, err = net.Listen("tcp", "127.0.0.1:0")
					return err
				},
				OnStop: func(context.Context) error {
					// Fx may call this after a preceding stop hook errors.
					// Leave the listener open while Run still uses it.
					if runner.Active() {
						return errors.New("TCP component still running; listener retained")
					}
					return listener.Close()
				},
			})
			var err error
			runner, err = xfx.New(lc, shutdowner, xrun.ComponentFunc(func(ctx context.Context) error {
				for {
					if ctx.Err() != nil {
						return nil
					}
					if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
						return err
					}
					conn, err := listener.Accept()
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						continue
					}
					if err != nil {
						return err
					}
					_ = conn.Close()
				}
			}))
			return err
		}),
	)
	if app.Err() != nil {
		panic(app.Err())
	}
	// Start, Wait, and Stop are controlled by the embedding application.
	// A successful OnStart means Run was launched, not that every service is ready.
	fmt.Println(runner != nil)
	// Output: true
}
