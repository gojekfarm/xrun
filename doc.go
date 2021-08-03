/*
Package xrun provides utilities around running multiple components
which are long-running components, example: an HTTP server or a background worker

	package main

	import (
		"net/http"
		"os"
		"os/signal"

		"github.com/gojekfarm/xrun"
		"github.com/gojekfarm/xrun/component"
	)

	func main() {
		m := xrun.NewManager()
		server := http.Server{
			Addr: ":9090",
		}
		_ = m.Add(component.HTTPServer(component.HTTPServerOptions{Server: &server}))

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		if err := m.Run(ctx); err != nil {
			os.Exit(1)
		}
	}
*/
package xrun
