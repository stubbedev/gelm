//go:build atspi

package main

import "github.com/stubbedev/lys-ui/app"

func initA11y(a *app.Application) { a.SetAccessibility(app.A11yOn) }
