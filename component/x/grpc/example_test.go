package grpc_test

import (
	"google.golang.org/grpc"

	xgrpc "github.com/gojekfarm/xrun/component/x/grpc"
)

func ExampleServer() {
	xgrpc.Server(xgrpc.Options{
		Server:      grpc.NewServer(),
		NewListener: xgrpc.NewListener(":8500"),
	})
}
