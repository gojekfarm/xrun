package component

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gojekfarm/xrun"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/nettest"
	"google.golang.org/grpc"
)

type GRPCServerSuite struct {
	suite.Suite
}

func TestGRPCServerSuite(t *testing.T) {
	suite.Run(t, new(GRPCServerSuite))
}

func (s *GRPCServerSuite) TestGRPCServer() {
	testcases := []struct {
		name                string
		wantErr             bool
		wantBadListener     bool
		wantShutdownTimeout bool
	}{
		{
			name: "SuccessfulStart",
		},
		{
			name:            "BadListener",
			wantBadListener: true,
			wantErr:         true,
		},
		{
			name:                "GracefulShutdownError",
			wantShutdownTimeout: true,
			wantErr:             true,
		},
	}

	for _, t := range testcases {
		s.Run(t.name, func() {
			var opts []xrun.Option
			if t.wantShutdownTimeout {
				opts = append(opts, xrun.WithGracefulShutdownTimeout(time.Nanosecond))
			}
			m := xrun.NewManager(opts...)
			srv := grpc.NewServer()

			var l net.Listener
			if t.wantBadListener {
				ml := &mockListener{}
				ml.On("Accept").Return(nil, errors.New("unknown listen error"))
				ml.On("Close").Return(nil)
				l = ml
				defer func() {
					ml.AssertExpectations(s.T())
				}()
			} else {
				var err error
				l, err = nettest.NewLocalListener("tcp")
				s.NoError(err)
			}

			s.NoError(m.Add(GRPCServer(GRPCServerOptions{
				Server: srv,
				Listener: l,
				PreStart: func() {
					s.T().Log("PreStart called")
				},
				PreStop: func() {
					s.T().Log("PreStop called")
				},
				PostStop: func() {
					s.T().Log("PostStop called")
				},
			})))

			errCh := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				errCh <- m.Start(ctx)
			}()

			time.Sleep(50 * time.Millisecond)

			cancel()
			if t.wantErr {
				s.Error(<-errCh)
			} else {
				s.NoError(<-errCh)
			}
		})
	}
}

type mockListener struct {
	mock.Mock
}

func (m *mockListener) Accept() (net.Conn, error) {
	args := m.Called()
	if err := args.Error(1); err != nil {
		return nil, err
	}
	return args.Get(0).(net.Conn), nil
}

func (m *mockListener) Close() error {
	return m.Called().Error(0)
}

func (m *mockListener) Addr() net.Addr {
	return m.Called().Get(0).(net.Addr)
}

type badAddr struct{}

func (b badAddr) Network() string {
	return "tcp"
}

func (b badAddr) String() string {
	return "bad address"
}
