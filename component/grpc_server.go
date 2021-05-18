package component

import (
	"context"
	"net"

	"github.com/gojekfarm/xrun"
	"google.golang.org/grpc"
)

type GRPCServerOptions struct {
	Server   *grpc.Server
	Listener net.Listener
	PreStart func()
	PreStop  func()
	PostStop func()
}

// GRPCServer is a helper which returns an xrun.ComponentFunc to start a grpc.Server
func GRPCServer(opts GRPCServerOptions) xrun.ComponentFunc {
	srv := opts.Server
	l := opts.Listener
	ps := opts.PreStart
	pst := opts.PreStop
	pstp := opts.PostStop

	return func(ctx context.Context) error {
		errCh := make(chan error, 1)
		go func() {
			if ps != nil {
				ps()
			}
			if err := srv.Serve(l); err != nil && err != grpc.ErrServerStopped {
				errCh <- err
			}
		}()

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
