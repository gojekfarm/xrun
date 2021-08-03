package xrun

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAll(t *testing.T) {
	r := All(NoTimeout, ComponentFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}))

	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		errCh <- r.Run(ctx)
	}()

	cancel()
	assert.NoError(t, <-errCh)
}
