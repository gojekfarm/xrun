package grpc

import (
	"context"
	"net"

	"github.com/gojekfarm/xrun"
	"google.golang.org/grpc"
)

// Options holds options for Server
type Options struct {
	Server   *grpc.Server
	Listener net.Listener
	PreStart func()
	PreStop  func()
	PostStop func()
}

// Server is a helper which returns a xrun.ComponentFunc to start a grpc.Server
func Server(opts Options) xrun.ComponentFunc {
	srv := opts.Server
	l := opts.Listener
	ps := opts.PreStart
	pst := opts.PreStop
	pstp := opts.PostStop

	return func(ctx context.Context) error {
		errCh := make(chan error, 1)

		go func(errCh chan error) {
			if ps != nil {
				ps()
			}

			if err := srv.Serve(l); err != nil && err != grpc.ErrServerStopped {
				errCh <- err
			}
		}(errCh)

		select {
		case <-ctx.Done():
		case err := <-errCh:
			return err
		}

		if pst != nil {
			pst()
		}

		srv.GracefulStop()

		if pstp != nil {
			pstp()
		}

		return nil
	}
}
