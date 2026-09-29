package component

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func preparedTestServer(t *testing.T, handler http.Handler, timeout time.Duration) *PreparedHTTPServer {
	t.Helper()
	p, err := NewPreparedHTTPServer(&http.Server{Addr: "127.0.0.1:0", Handler: handler}, timeout)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = p.Close(ctx)
	})
	return p
}

func waitResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("component did not return")
		return nil
	}
}

func TestPreparedHTTPServerBindReadinessAndIndependentInstances(t *testing.T) {
	first := preparedTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "first")
	}), time.Second)
	second := preparedTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "second")
	}), time.Second)
	if first.Addr().String() == second.Addr().String() {
		t.Fatal("separate instances shared an address")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := []chan error{make(chan error, 1), make(chan error, 1)}
	go func() { results[0] <- first.Run(ctx) }()
	go func() { results[1] <- second.Run(ctx) }()
	for _, test := range []struct {
		server *PreparedHTTPServer
		want   string
	}{{first, "first"}, {second, "second"}} {
		resp, err := http.Get("http://" + test.server.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || string(body) != test.want {
			t.Fatalf("body = %q, err = %v", body, err)
		}
	}
	cancel()
	for _, result := range results {
		if err := waitResult(t, result); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPreparedHTTPServerBindFailureAndRollback(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	p, err := NewPreparedHTTPServer(&http.Server{Addr: occupied.Addr().String()}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Prepare(context.Background()); err == nil || p.Addr() != nil {
		t.Fatalf("bind result = %v, address = %v", err, p.Addr())
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	check, err := net.Listen("tcp", occupied.Addr().String())
	if err != nil {
		t.Fatalf("rollback left listener open: %v", err)
	}
	_ = check.Close()
}

func TestPreparedHTTPServerCancellationDrainsStreamingHandler(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	p := preparedTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	}), time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	drainResult := make(chan error, 1)
	go func() { runResult <- p.Run(ctx) }()
	go func() { drainResult <- p.HandlerDrain().Run(ctx) }()
	resp, err := http.Get("http://" + p.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	select {
	case err := <-drainResult:
		t.Fatalf("handler drain returned before stream ended: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if err := waitResult(t, drainResult); err != nil {
		t.Fatal(err)
	}
	if err := waitResult(t, runResult); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedHTTPServerFatalServeDoesNotWaitForIgnoredHandler(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	p := preparedTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release // Deliberately ignores request cancellation.
		w.WriteHeader(http.StatusNoContent)
	}), 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runResult := make(chan error, 1)
	drainResult := make(chan error, 1)
	go func() { runResult <- p.Run(ctx) }()
	go func() { drainResult <- p.HandlerDrain().Run(ctx) }()
	requestDone := make(chan struct{})
	go func() {
		resp, err := http.Get("http://" + p.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	<-started
	if err := p.listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := waitResult(t, runResult); err == nil || !strings.Contains(err.Error(), "serve HTTP") {
		t.Fatalf("fatal listener error = %v", err)
	}
	cancel()
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer closeCancel()
	if err := p.Close(closeCtx); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close with ignored handler = %v", err)
	}
	select {
	case <-drainResult:
		t.Fatal("handler drain returned before ignored handler ended")
	default:
	}
	close(release)
	if err := waitResult(t, drainResult); err != nil {
		t.Fatal(err)
	}
	<-requestDone
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedHTTPServerInvalidConstruction(t *testing.T) {
	if _, err := NewPreparedHTTPServer(nil, time.Second); err == nil {
		t.Fatal("nil server accepted")
	}
	if _, err := NewPreparedHTTPServer(&http.Server{}, 0); err == nil {
		t.Fatal("zero timeout accepted")
	}
}
