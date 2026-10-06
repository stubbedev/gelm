//go:build atspi

package app

import "github.com/stubbedev/gelm/internal/atspi"

// A11YOptions configures the AT-SPI bridge behind ServeAccessibility
// (app/atspi.go). The alias sits beside the bridge behind the `atspi`
// build tag, but unlike internal/atspi it is exported, so applications
// outside gelm can name and construct the zero value once they build
// with the tag.
type A11YOptions = atspi.Options
