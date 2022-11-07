package grpc_test

import (
	xgrpc "github.com/gojekfarm/xrun/component/x/grpc"
	"google.golang.org/grpc"
)

func ExampleServer() {
	xgrpc.Server(xgrpc.Options{
		Server:      grpc.NewServer(),
		NewListener: xgrpc.NewListener(":8500"),
	})
}
