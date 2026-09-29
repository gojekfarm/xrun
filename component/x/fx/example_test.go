package fx_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/fx"

	"github.com/gojekfarm/xrun"
	xfx "github.com/gojekfarm/xrun/component/x/fx"
)

type exampleServer struct {
	address  string
	listener net.Listener
}

func (s *exampleServer) Prepare(context.Context) error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	s.listener = listener

	return nil
}

func (s *exampleServer) Run(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

func (s *exampleServer) Close(context.Context) error { return s.listener.Close() }

type exampleAddresses struct {
	fx.In
	Public string `name:"public"`
	Admin  string `name:"admin"`
}

type exampleServers struct {
	fx.Out
	Public     *exampleServer   `name:"public"`
	Admin      *exampleServer   `name:"admin"`
	Components []xrun.Component `group:"xrun,flatten"`
}

func newExampleServers(in exampleAddresses) exampleServers {
	public := &exampleServer{address: in.Public}
	admin := &exampleServer{address: in.Admin}

	return exampleServers{
		Public: public, Admin: admin,
		Components: []xrun.Component{public, admin},
	}
}

// Both named values and the component group refer to the same two instances.
// Each instance binds its own listener during Fx startup and closes it after
// its Run call has finished.
func ExampleModule() {
	app := fx.New(
		fx.NopLogger,
		fx.StopTimeout(5*time.Second),
		fx.Provide(
			fx.Annotate(func() string { return "127.0.0.1:0" }, fx.ResultTags(`name:"public"`)),
			fx.Annotate(func() string { return "127.0.0.1:0" }, fx.ResultTags(`name:"admin"`)),
			newExampleServers,
		),
		xfx.Module(),
	)
	if err := app.Err(); err != nil {
		panic(err)
	}
	if err := app.Start(context.Background()); err != nil {
		panic(err)
	}
	if err := app.Stop(context.Background()); err != nil {
		panic(err)
	}

	fmt.Println("two named servers stopped")
	// Output: two named servers stopped
}
