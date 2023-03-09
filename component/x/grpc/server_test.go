package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gojekfarm/xrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/nettest"
	"google.golang.org/grpc"
)

type ServerTestSuite struct {
	suite.Suite
}

func TestServerSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

func (s *ServerTestSuite) TestServer() {
	testcases := []struct {
		name                string
		wantErr             bool
		newListener         func() (net.Listener, error)
		wantShutdownTimeout bool
	}{
		{
			name: "SuccessfulStart",
			newListener: func() (net.Listener, error) {
				l, _ := nettest.NewLocalListener("tcp")
				return l, nil
			},
		},
		{
			name: "BadListener",
			newListener: func() (net.Listener, error) {
				ml := &mockListener{}
				ml.On("Accept").Return(nil, errors.New("unknown listen error"))
				ml.On("Close").Return(nil)
				return ml, nil
			},
			wantErr: true,
		},
		{
			name: "GracefulShutdownError",
			newListener: func() (net.Listener, error) {
				l, _ := nettest.NewLocalListener("tcp")
				return l, nil
			},
			wantShutdownTimeout: true,
			wantErr:             true,
		},
		{
			name: "ListenerCreateError",
			newListener: func() (net.Listener, error) {
				return nil, errors.New("cannot create a listener")
			},
			wantErr: true,
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

			l, err := t.newListener()
			st := s.T()

			s.NoError(m.Add(Server(Options{
				Server: srv,
				NewListener: func() (net.Listener, error) {
					return l, err
				},
				PreStart: func() { st.Log("PreStart called") },
				PreStop:  func() { st.Log("PreStop called") },
				PostStop: func() { st.Log("PostStop called") },
			})))

			errCh := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				errCh <- m.Run(ctx)
			}()

			time.Sleep(50 * time.Millisecond)

			cancel()
			if t.wantErr {
				s.Error(<-errCh)
			} else {
				s.NoError(<-errCh)
			}

			if ml, ok := l.(*mockListener); ok {
				ml.AssertExpectations(s.T())
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

func TestNewListener(t *testing.T) {
	f := NewListener(":0")
	l, err := f()

	assert.NoError(t, err)
	assert.NotNil(t, l)
}
