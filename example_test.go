package xrun_test

import (
	"context"
	"net/http"
	"os"
	"os/signal"

	"github.com/gojekfarm/xrun"
	"github.com/gojekfarm/xrun/component"
)

func ExampleNewManager() {
	m := xrun.NewManager(xrun.ShutdownTimeout(xrun.NoTimeout))

	if err := m.Add(component.HTTPServer(component.HTTPServerOptions{Server: &http.Server{}})); err != nil {
		panic(err)
	}

	if err := m.Add(xrun.ComponentFunc(func(ctx context.Context) error {
		// Start something here in a blocking way and continue on ctx.Done
		<-ctx.Done()
		// Call Stop on component if cleanup is required
		return nil
	})); err != nil {
		panic(err)
	}

	// ctx is marked done (its Done channel is closed) when one of the listed signals arrives
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	if err := m.Run(ctx); err != nil {
		os.Exit(1)
	}
}

func ExampleNewManager_nested() {
	m1 := xrun.NewManager()
	if err := m1.Add(component.HTTPServer(component.HTTPServerOptions{Server: &http.Server{}})); err != nil {
		panic(err)
	}

	m2 := xrun.NewManager()
	if err := m2.Add(xrun.ComponentFunc(func(ctx context.Context) error {
		// Start something here in a blocking way and continue on ctx.Done
		<-ctx.Done()
		// Call Stop on component if cleanup is required
		return nil
	})); err != nil {
		panic(err)
	}

	gm := xrun.NewManager()
	if err := gm.Add(m1); err != nil {
		panic(err)
	}
	if err := gm.Add(m2); err != nil {
		panic(err)
	}

	// ctx is marked done (its Done channel is closed) when one of the listed signals arrives
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	// Run will start m1 and m2 simultaneously
	if err := gm.Run(ctx); err != nil {
		os.Exit(1)
	}
}

func ExampleAll() {
	// ctx is marked done (its Done channel is closed) when one of the listed signals arrives
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	if err := xrun.All(xrun.NoTimeout,
		component.HTTPServer(component.HTTPServerOptions{Server: &http.Server{}}),
		xrun.ComponentFunc(func(ctx context.Context) error {
			// Start something here in a blocking way and continue on ctx.Done
			<-ctx.Done()
			// Call Stop on component if cleanup is required
			return nil
		}),
	).Run(ctx); err != nil {
		os.Exit(1)
	}
}
