//go:build atspi

// The application side of the AT-SPI bridge (#65): ServeAccessibility
// turns the application into the bridge's Scene — window roots, the
// focused widget of the topmost window, and the loop queue — and
// starts serving the semantic model on the accessibility bus. The file
// (like internal/atspi) is behind the build tag, so default builds
// carry none of it.
package app

import (
	"github.com/stubbedev/gelm/internal/atspi"
	"github.com/stubbedev/gelm/widget"
)

// ServeAccessibility starts the in-process AT-SPI bridge: the
// accessibility bus is discovered through org.a11y.Bus (opts
// overrides), the window trees are exported at the AT-SPI root, and
// the registry handshake runs. Idempotent. Call before Run (the same
// window SetRoot et al. honor) or from the loop; the bridge stops with
// Run. The first sample queues through Invoke and drains once Run
// starts. opts is A11YOptions (the untagged alias) so consumers
// outside gelm can construct it.
func (a *Application) ServeAccessibility(opts A11YOptions) error {
	if a.stopA11y != nil {
		return nil
	}
	br, err := atspi.Serve(a, opts)
	if err != nil {
		return err
	}
	a.stopA11y = br.Stop
	return nil
}

// Roots implements atspi.Scene: the window roots in window order.
// Only ever called on the loop goroutine (from inside Invoke), where
// the window list and the routers are consistent.
func (a *Application) Roots() []widget.Widget {
	out := make([]widget.Widget, 0, len(a.windows))
	for _, w := range a.windows {
		out = append(out, w.router.Root)
	}
	return out
}

// Focused implements atspi.Scene: the focused widget of the topmost
// window (the most recently created one holds input focus), nil when
// nothing does.
func (a *Application) Focused() widget.Widget {
	for i := len(a.windows) - 1; i >= 0; i-- {
		if f := a.windows[i].router.Focused(); f != nil {
			return f
		}
	}
	return nil
}
