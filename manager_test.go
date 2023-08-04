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
	type testcase struct {
		name       string
		wantErr    assert.ErrorAssertionFunc
		wantAddErr bool
		components []Component
		wantOrder  []int
		options    []Option
	}

	s.Run("DefaultManager", func() {
		testcases := []testcase{
			{
				name:    "WithZeroComponents",
				wantErr: assert.NoError,
			},
			{
				name:    "WithOneComponent",
				wantErr: assert.NoError,
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
				wantErr: assert.Error,
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						return errors.New("start error")
					}),
				},
			},
			{
				name:    "WithGracefulShutdownErrorOnOneComponent",
				options: []Option{ShutdownTimeout(5 * time.Second)},
				wantErr: assert.Error,
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
				options: []Option{ShutdownTimeout(time.Minute)},
				wantErr: assert.NoError,
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
				wantErr: assert.NoError,
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						time.Sleep(5 * time.Second)
						<-ctx.Done()
						time.Sleep(5 * time.Second)
						return nil
					}),
				},
			},
			{
				name:    "ShutdownWhenComponentReturnsContextErrorAsItIs",
				wantErr: assert.NoError,
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						time.Sleep(time.Second)
						<-ctx.Done()
						time.Sleep(2 * time.Second)
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						time.Sleep(time.Second)
						<-ctx.Done()
						time.Sleep(time.Second)
						return ctx.Err()
					}),
				},
			},
			{
				name: "ShutdownWhenOneComponentReturnsErrorOnExit",
				wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
					return assert.EqualError(t, err, `1 error occurred:
	* shutdown error

`, i...)
				},
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						time.Sleep(time.Second)
						<-ctx.Done()
						time.Sleep(2 * time.Second)
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						time.Sleep(time.Second)
						<-ctx.Done()
						time.Sleep(time.Second)
						return errors.New("shutdown error")
					}),
				},
			},
			{
				name: "ShutdownWhenMoreThanOneComponentReturnsErrorOnExit",
				wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
					return assert.EqualError(t, err, `2 errors occurred:
	* shutdown error 2
	* shutdown error 1

`, i...)
				},
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						<-ctx.Done()
						time.Sleep(2 * time.Second)
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						<-ctx.Done()
						time.Sleep(3 * time.Second)
						return errors.New("shutdown error 1")
					}),
					ComponentFunc(func(ctx context.Context) error {
						<-ctx.Done()
						time.Sleep(2 * time.Second)
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

				time.Sleep(1 * time.Second)
				cancel()

				t.wantErr(s.T(), <-errCh)
			})
		}
	})

	s.Run("OrderedStart", func() {
		testcases := []testcase{
			{
				name:      "OrderedStartWithSignalStartedCalled",
				wantOrder: []int{1, 2, 3},
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						SignalStarted(ctx)
						<-ctx.Done()
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						SignalStarted(ctx)
						<-ctx.Done()
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						SignalStarted(ctx)
						<-ctx.Done()
						return nil
					}),
				},
				wantErr: assert.NoError,
			},
			{
				name:      "OrderedStartWithSignalStartedNotCalled",
				wantOrder: []int{1, 2, 3},
				options:   []Option{MaxStartWait(100 * time.Millisecond)},
				components: []Component{
					ComponentFunc(func(ctx context.Context) error {
						SignalStarted(ctx)
						<-ctx.Done()
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					}),
					ComponentFunc(func(ctx context.Context) error {
						SignalStarted(ctx)
						<-ctx.Done()
						return nil
					}),
				},
				wantErr: assert.NoError,
			},
		}

		for _, t := range testcases {
			s.Run(t.name, func() {
				m := NewManager(append(t.options, OrderedStart)...)

				var order []int
				for i, r := range t.components {
					ii := i
					rr := r
					s.NoError(m.Add(ComponentFunc(func(ctx context.Context) error {
						order = append(order, ii+1)
						return rr.Run(ctx)
					})))
				}

				ctx, cancel := context.WithCancel(context.Background())

				errCh := make(chan error, 1)
				go func() {
					errCh <- m.Run(ctx)
				}()

				time.Sleep(1 * time.Second)
				cancel()

				t.wantErr(s.T(), <-errCh)
				s.Equal(t.wantOrder, order)
			})
		}
	})
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
