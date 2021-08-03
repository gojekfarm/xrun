/*
Package component contains some commonly used implementations
of long-running components like an HTTP server.

	package component

	import (
		"context"
		"net"

		"github.com/gojekfarm/xrun"
		"google.golang.org/grpc"
	)

	// GRPCServerOptions holds options for GRPCServer
	type GRPCServerOptions struct {
		Server   *grpc.Server
		Listener net.Listener
	}

	// GRPCServer is a helper which returns an xrun.ComponentFunc to run a grpc.Server
	func GRPCServer(opts GRPCServerOptions) xrun.ComponentFunc {
		srv := opts.Server
		l := opts.Listener

		return func(ctx context.Context) error {
			errCh := make(chan error, 1)
			go func() {
				if err := srv.Serve(l); err != nil && err != grpc.ErrServerStopped {
					errCh <- err
				}
			}()

			select {
			case <-ctx.Done():
			case err := <-errCh:

				return err
			}

			srv.GracefulStop()

			return nil
		}
	}

*/
package component
