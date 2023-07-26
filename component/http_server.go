package component

import (
	"context"
	"net/http"

	"github.com/gojekfarm/xrun"
)

// HTTPServerOptions holds options for HTTPServer
type HTTPServerOptions struct {
	Server   *http.Server
	PreStart func()
	PreStop  func()
}

// HTTPServer is a helper which returns an xrun.ComponentFunc to start an http.Server
func HTTPServer(opts HTTPServerOptions) xrun.ComponentFunc {
	srv := opts.Server
	ps := opts.PreStart
	pst := opts.PreStop

	return func(ctx context.Context) error {
		errCh := make(chan error, 1)

		go func() {
			if ps != nil {
				ps()
			}

			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()

		select {
		case <-ctx.Done():
		case err := <-errCh:
			return err
		}

		shutdownCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if pst != nil {
			pst()
		}

		return srv.Shutdown(shutdownCtx)
	}
}
