package component

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gojekfarm/xrun"
)

// PreparedHTTPServer owns a listener and an HTTP server. It implements the
// prepared component contract used by component/x/fx without depending on Fx.
// Register HandlerDrain alongside it in the same supervisor whenever handlers
// use resources owned by other prepared components. Construct it before
// starting the server and do not mutate Server afterward.
type PreparedHTTPServer struct {
	server          *http.Server
	shutdownTimeout time.Duration

	mu       sync.Mutex
	listener net.Listener
	prepared bool
	started  bool
	closed   bool
	draining bool
	active   int
	drained  chan struct{}
	runDone  chan struct{}
}

// NewPreparedHTTPServer takes exclusive ownership of server. Its handler is
// wrapped to track in-flight requests, including streaming handlers. A nil
// handler uses http.DefaultServeMux, as in net/http. shutdownTimeout must be
// positive and bounds both graceful shutdown and Close when its context has no
// earlier deadline.
func NewPreparedHTTPServer(server *http.Server, shutdownTimeout time.Duration) (*PreparedHTTPServer, error) {
	if server == nil {
		return nil, errors.New("HTTP server is required")
	}
	if shutdownTimeout <= 0 {
		return nil, errors.New("positive HTTP shutdown timeout is required")
	}
	p := &PreparedHTTPServer{server: server, shutdownTimeout: shutdownTimeout, drained: make(chan struct{}), runDone: make(chan struct{})}
	handler := server.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}
	server.Handler = p.track(handler)
	return p, nil
}

func (p *PreparedHTTPServer) track(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		if p.draining {
			p.mu.Unlock()
			http.Error(w, "server stopping", http.StatusServiceUnavailable)
			return
		}
		p.active++
		p.mu.Unlock()
		defer func() {
			p.mu.Lock()
			p.active--
			if p.draining && p.active == 0 {
				close(p.drained)
			}
			p.mu.Unlock()
		}()
		handler.ServeHTTP(w, r)
	})
}

// Prepare binds synchronously, so a successful return means the address is
// reserved. The listener belongs to this component until Close. Failed binds
// leave no acquired resources and can be retried.
func (p *PreparedHTTPServer) Prepare(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.prepared || p.closed {
		return errors.New("HTTP server already prepared or closed")
	}
	addr := p.server.Addr
	if addr == "" {
		addr = ":http"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("bind HTTP listener: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, listener.Close())
	}
	p.listener = listener
	p.prepared = true
	return nil
}

// Addr returns the bound address after Prepare, or nil before preparation.
func (p *PreparedHTTPServer) Addr() net.Addr {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener == nil {
		return nil
	}
	return p.listener.Addr()
}

// HandlerDrain is a second supervised component that keeps the supervisor's
// join gate active until all accepted handlers return. Register it alongside
// p when another prepared component owns resources used by HTTP handlers.
// A stop deadline may expire while a handler ignores cancellation; in that
// case the Fx bridge deliberately skips closing every prepared owner.
func (p *PreparedHTTPServer) HandlerDrain() xrun.Component {
	return xrun.ComponentFunc(func(ctx context.Context) error {
		<-ctx.Done()
		p.beginDrain()
		<-p.drained
		return nil
	})
}

// Run serves until its context is canceled or Serve exits. The context is used
// as the base context for accepted connections, so cancellation reaches active
// requests. A Serve failure is returned immediately, even if a handler ignores
// cancellation; Close subsequently handles the bounded handler drain.
func (p *PreparedHTTPServer) Run(ctx context.Context) error {
	p.mu.Lock()
	if !p.prepared || p.closed || p.started {
		p.mu.Unlock()
		return errors.New("HTTP server is not prepared or already running")
	}
	p.started = true
	listener := p.listener
	p.server.BaseContext = func(net.Listener) context.Context { return ctx }
	p.mu.Unlock()
	defer close(p.runDone)

	serveErr := make(chan error, 1)
	go func() { serveErr <- p.server.Serve(listener) }()
	select {
	case err := <-serveErr:
		if ctx.Err() != nil && errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}
	p.beginDrain()
	err := p.stopServer(ctx)
	if serveError := <-serveErr; !errors.Is(serveError, http.ErrServerClosed) {
		err = errors.Join(err, fmt.Errorf("serve HTTP: %w", serveError))
	}
	return err
}

func (p *PreparedHTTPServer) beginDrain() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.draining {
		return
	}
	p.draining = true
	if p.active == 0 {
		close(p.drained)
	}
}

func (p *PreparedHTTPServer) stopServer(parent context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), p.shutdownTimeout)
	// The caller's deadline can only shorten the configured drain budget.
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) < p.shutdownTimeout {
		cancel()
		shutdownCtx, cancel = context.WithDeadline(context.Background(), deadline)
	}
	defer cancel()
	if err := p.server.Shutdown(shutdownCtx); err != nil {
		return errors.Join(fmt.Errorf("shutdown HTTP: %w", err), p.server.Close())
	}
	return nil
}

// Close releases the listener after Run has returned and every accepted
// handler has returned. If a handler ignores cancellation past the deadline,
// Close returns an error and can be retried; callers must not close resources
// used by handlers until Close succeeds. Close also supports rollback after a
// successful Prepare when Run never started.
func (p *PreparedHTTPServer) Close(ctx context.Context) error {
	closeCtx, cancel := context.WithTimeout(ctx, p.shutdownTimeout)
	defer cancel()
	p.mu.Lock()
	if !p.prepared || p.closed {
		p.mu.Unlock()
		return nil
	}
	started := p.started
	p.mu.Unlock()
	if started {
		select {
		case <-p.runDone:
		case <-closeCtx.Done():
			return fmt.Errorf("HTTP run still active: %w", closeCtx.Err())
		}
	}
	p.beginDrain()
	var err error
	if started {
		err = p.stopServer(closeCtx)
	} else {
		err = p.server.Close()
	}
	select {
	case <-p.drained:
	case <-closeCtx.Done():
		return errors.Join(err, fmt.Errorf("HTTP handlers still active: %w", closeCtx.Err()))
	}
	p.mu.Lock()
	p.closed = true
	listener := p.listener
	p.mu.Unlock()
	if listener != nil {
		if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}
