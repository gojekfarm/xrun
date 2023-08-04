package xrun

import (
	"context"
	"testing"
)

func Test_started_with_no_notifyCtx(t *testing.T) {
	<-started(context.TODO())
}
