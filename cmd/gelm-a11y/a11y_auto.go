//go:build !atspi

package main

import "github.com/stubbedev/gelm/app"

func initA11y(a *app.Application) { a.SetAccessibility(app.A11yAuto) }
