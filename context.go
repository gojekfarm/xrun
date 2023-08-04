package xrun

import (
	"context"
	"sync/atomic"
)

// SignalStarted signals that the component has started.
func SignalStarted(ctx context.Context) {
	if n, ok := ctx.Value(ctxKeyNotify).(*notifyCtx); ok {
		n.start()
	}
}

type ctxKey int

const (
	ctxKeyNotify ctxKey = iota
)

type notifyCtx struct {
	parent   context.Context
	_started atomic.Bool
	ch       chan struct{}
}

func newNotifyCtx(parent context.Context) context.Context {
	return context.WithValue(parent, ctxKeyNotify, &notifyCtx{
		parent: parent,
		ch:     make(chan struct{}, 1),
	})
}

func (n *notifyCtx) started() <-chan struct{} { return n.ch }

func (n *notifyCtx) start() {
	if n._started.CompareAndSwap(false, true) {
		close(n.ch)
	}
}

func started(ctx context.Context) <-chan struct{} {
	if n, ok := ctx.Value(ctxKeyNotify).(*notifyCtx); ok {
		return n.started()
	}

	return closedCh()
}

func closedCh() <-chan struct{} {
	ch := make(chan struct{}, 1)
	defer close(ch)

	return ch
}
