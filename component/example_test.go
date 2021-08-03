package component_test

import (
	"context"
	"fmt"
	"github.com/gojekfarm/xrun/component"
	"net/http"
	"os"
	"os/signal"
)

func ExampleHTTPServer() {
	c := component.HTTPServer(component.HTTPServerOptions{
		Server: &http.Server{},
		PreStart: func() {
			fmt.Println("starting server")
		},
		PreStop: func() {
			fmt.Println("stopping server")
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	if err := c.Run(ctx); err != nil {
		os.Exit(1)
	}
}
