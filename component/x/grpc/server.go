package grpc

import (
	"context"
	"errors"
	"net"

	"google.golang.org/grpc"

	"github.com/gojekfarm/xrun"
)

// Options holds options for Server
type Options struct {
	Server      *grpc.Server
	NewListener func() (net.Listener, error)
	PreStart    func()
	PreStop     func()
	PostStop    func()
}

// Server is a helper which returns a xrun.ComponentFunc to start a grpc.Server
func Server(opts Options) xrun.ComponentFunc {
	srv := opts.Server
	nl := opts.NewListener
	ps := opts.PreStart
	pst := opts.PreStop
	pstp := opts.PostStop

	return func(ctx context.Context) error {
		l, err := nl()
		if err != nil {
			return err
		}

		errCh := make(chan error, 1)

		go func(errCh chan error) {
			if ps != nil {
				ps()
			}

			if err := srv.Serve(l); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
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

func NewListener(address string) func() (net.Listener, error) {
	return func() (net.Listener, error) {
		return net.Listen("tcp", address)
	}
}
