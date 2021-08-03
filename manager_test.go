package xrun

import (
	"context"
	"errors"
	"testing"
	"time"

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
		wantErr    bool
		wantAddErr bool
		components []Component
		options    []Option
	}{
		{
			name:    "WithZeroComponents",
			wantErr: false,
		},
		{
			name:    "WithOneComponent",
			wantErr: false,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(3 * time.Second)
					<-ctx.Done()
					return nil
				}),
			},
		},
		{
			name:    "WithErrorOnComponentStart",
			wantErr: true,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					return errors.New("start error")
				}),
			},
		},
		{
			name:    "WithGracefulShutdownErrorOnOneComponent",
			options: []Option{WithGracefulShutdownTimeout(5 * time.Second)},
			wantErr: true,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(time.Second)
					<-ctx.Done()
					time.Sleep(time.Second)
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
			options: []Option{WithGracefulShutdownTimeout(time.Minute)},
			wantErr: false,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(5 * time.Second)
					<-ctx.Done()
					time.Sleep(5 * time.Second)
					return nil
				}),
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(time.Second)
					<-ctx.Done()
					time.Sleep(10 * time.Second)
					return nil
				}),
			},
		},
		{
			name:    "UndefinedGracefulShutdown",
			wantErr: false,
			components: []Component{
				ComponentFunc(func(ctx context.Context) error {
					time.Sleep(5 * time.Second)
					<-ctx.Done()
					time.Sleep(5 * time.Second)
					return nil
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

			time.Sleep(1 * time.Second)
			cancel()

			if err := <-errCh; t.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
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

	time.Sleep(1 * time.Second)
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

	time.Sleep(1 * time.Second)

	s.EqualError(m.Add(ComponentFunc(func(ctx context.Context) error {
		return nil
	})), "can't accept new component as manager has already started")
	cancel()

	s.NoError(<-errCh)
}
