package xrun

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithGracefulShutdownTimeout(t *testing.T) {
	expected := time.Minute

	m := NewManager(WithGracefulShutdownTimeout(expected))
	assert.Equal(t, expected, m.(*manager).shutdownTimeout)
}
