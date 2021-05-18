/*
Package xrun provides utilities around starting multiple components
which are long running components, example: an HTTP server or a background worker

	package main

	import (
		"net/http"
		"os"

		"github.com/gojekfarm/xrun"
		"github.com/gojekfarm/xrun/component"
		"github.com/gojekfarm/xrun/signals"
	)

	func main() {
		m := xrun.NewManager()
		server := http.Server{
			Addr: ":9090",
		}
		_ = m.Add(component.HTTPServer(component.HTTPServerOptions{Server: &server}))

		if err := m.Start(signals.OSSignalHandler()); err != nil {
			os.Exit(1)
		}
	}
*/
package xrun
