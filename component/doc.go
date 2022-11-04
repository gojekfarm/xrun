/*
Package component contains some commonly used implementations
of long-running components like an HTTP server.

	package main

	import (
		"context"
		"fmt"
		"net/http"
		"os"
		"os/signal"

		"github.com/gojekfarm/xrun/component"
	)

	func main() {
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

*/
package component
