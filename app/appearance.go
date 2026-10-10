package app

import (
	"sync"
	"sync/atomic"

	"github.com/stubbedev/gelm/appearance"
)

// ColorScheme returns the desktop's current dark/light preference,
// appearance.Unknown without a portal. It is valid from NewApplication
// on, so the first frame can be themed from it.
func (a *Application) ColorScheme() appearance.ColorScheme { return a.portal.ColorScheme() }

// OnColorSchemeChange registers fn for every color-scheme change. fn
// runs on the loop goroutine, so it may call widget.SetTheme directly.
// The returned function unregisters it; a change already queued when it
// runs is dropped.
func (a *Application) OnColorSchemeChange(fn func(appearance.ColorScheme)) (off func()) {
	return onLoop(a, a.portal.OnColorSchemeChange, fn)
}

// Accent returns the desktop's accent-color preference.
func (a *Application) Accent() appearance.Accent { return a.portal.Accent() }

// OnAccentChange registers fn for every accent-color change, on the
// loop goroutine like OnColorSchemeChange.
func (a *Application) OnAccentChange(fn func(appearance.Accent)) (off func()) {
	return onLoop(a, a.portal.OnAccentChange, fn)
}

// Contrast returns the desktop's contrast preference.
func (a *Application) Contrast() appearance.Contrast { return a.portal.Contrast() }

// OnContrastChange registers fn for every contrast change, on the loop
// goroutine like OnColorSchemeChange.
func (a *Application) OnContrastChange(fn func(appearance.Contrast)) (off func()) {
	return onLoop(a, a.portal.OnContrastChange, fn)
}

func onLoop[T any](a *Application, register func(func(T)) func(), fn func(T)) (off func()) {
	var stopped atomic.Bool
	unregister := register(func(v T) {
		a.Invoke(func() {
			if !stopped.Load() {
				fn(v)
			}
		})
	})
	return sync.OnceFunc(func() {
		stopped.Store(true)
		unregister()
	})
}
