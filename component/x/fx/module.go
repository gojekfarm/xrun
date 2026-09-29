package fx

import (
	"context"

	uberfx "go.uber.org/fx"

	"github.com/gojekfarm/xrun"
)

// PreparedComponent is an optional lifecycle contract for components that
// acquire resources or need to report readiness failures during Fx startup.
// Prepare runs synchronously before any component Run starts. A failed Prepare
// must release its own partial acquisitions. After every Run has returned,
// Close is called once for each successfully prepared component in reverse
// preparation order. Close must respect its context and release resources even
// after cancellation. If Run does not return before the stop deadline, Close
// is not called because the component may still use those resources.
type PreparedComponent interface {
	xrun.Component
	Prepare(context.Context) error
	Close(context.Context) error
}

type moduleRunner struct{}

// Module supervises xrun.Component values supplied in the Fx group "xrun".
// Use fx.Annotate with fx.As(new(xrun.Component)) and
// fx.ResultTags(`group:"xrun"`) to provide a component. A constructor may also
// export the same concrete instance by Fx name for other consumers.
// Fx value-group order is unspecified; components needing ordered preparation
// should be combined into one PreparedComponent.
func Module() uberfx.Option {
	type params struct {
		uberfx.In
		Lifecycle  uberfx.Lifecycle
		Shutdowner uberfx.Shutdowner
		Components []xrun.Component `group:"xrun"`
	}

	return uberfx.Options(
		uberfx.Provide(func(p params) (moduleRunner, error) {
			_, err := New(p.Lifecycle, p.Shutdowner, p.Components...)

			return moduleRunner{}, err
		}),
		uberfx.Invoke(func(moduleRunner) {}),
	)
}
