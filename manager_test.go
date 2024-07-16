package xrun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ManagerSuite struct {
	suite.Suite
}

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}

func (s *ManagerSuite) TestNewManager() {
	testcases := []struct {
		name       string
		wantErr    assert.ErrorAssertionFunc
		wantAddErr bool
		components []Component
		options    []Option
	}{
		{
			name:    "WithZeroComponents",
			wantErr: assert.NoError,
		},
		{
			name:    "WithOneComponent",
			wantErr: assert.NoError,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(300 * time.Millisecond)
					<-ctx.Done()
					return nil
				}),
			},
		},
		{
			name:    "WithErrorOnComponentStart",
			wantErr: assert.Error,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					return errors.New("start error")
				}),
			},
		},
		{
			name:    "WithGracefulShutdownErrorOnOneComponent",
			options: []Option{ShutdownTimeout(time.Second)},
			wantErr: assert.Error,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(100 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(100 * time.Millisecond)
					return nil
				}),
				ComponentFunc(func(ctx context.Context) error {
					<-ctx.Done()
					time.Sleep(time.Minute)
					return nil
				}),
			},
		},
		{
			name:    "WithGracefulShutdownForTwoLongRunningComponents",
			options: []Option{ShutdownTimeout(time.Minute)},
			wantErr: assert.NoError,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(500 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(500 * time.Millisecond)
					return nil
				}),
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(100 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(time.Second)
					return nil
				}),
			},
		},
		{
			name:    "UndefinedGracefulShutdown",
			wantErr: assert.NoError,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					<-ctx.Done()
					time.Sleep(2 * time.Second)
					return nil
				}),
			},
		},
		{
			name:    "ShutdownWhenComponentReturnsContextErrorAsItIs",
			wantErr: assert.NoError,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(100 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(200 * time.Millisecond)
					return nil
				}),
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(100 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(100 * time.Millisecond)
					return ctx.Err()
				}),
			},
		},
		{
			name: "ShutdownWhenOneComponentReturnsErrorOnExit",
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.EqualError(t, err, "shutdown error", i...)
			},
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(100 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(200 * time.Millisecond)
					return nil
				}),
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(100 * time.Millisecond)
					<-ctx.Done()
					time.Sleep(100 * time.Millisecond)
					return errors.New("shutdown error")
				}),
			},
		},
		{
			name: "ShutdownWhenMoreThanOneComponentReturnsErrorOnExit",
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.EqualError(t, err, `shutdown error 2
shutdown error 1`, i...)
			},
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					<-ctx.Done()
					time.Sleep(200 * time.Millisecond)
					return nil
				}),
				ComponentFunc(func(ctx context.Context) error {
					<-ctx.Done()
					time.Sleep(300 * time.Millisecond)
					return errors.New("shutdown error 1")
				}),
				ComponentFunc(func(ctx context.Context) error {
					<-ctx.Done()
					time.Sleep(200 * time.Millisecond)
					return errors.New("shutdown error 2")
				}),
			},
		},
	}

	for _, t := range testcases {
		s.Run(t.name, func() {
			m := NewManager(t.options...)

			for _, r := range t.components {
				s.NoError(m.Add(r))
			}

			ctx, cancel := context.WithCancel(context.Background())

			errCh := make(chan error, 1)
			go func() {
				errCh <- m.Run(ctx)
			}()

			time.Sleep(300 * time.Millisecond)
			cancel()

			t.wantErr(s.T(), <-errCh)
		})
	}
}

func (s *ManagerSuite) TestAddNewComponentAfterStop() {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	s.NoError(<-errCh)

	s.EqualError(m.Add(ComponentFunc(func(ctx context.Context) error {
		return nil
	})), "can't accept new component as stop procedure is already engaged")
}

func (s *ManagerSuite) TestAddNewComponentAfterStart() {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	s.EqualError(m.Add(ComponentFunc(func(ctx context.Context) error {
		return nil
	})), "can't accept new component as manager has already started")
	cancel()

	s.NoError(<-errCh)
}
