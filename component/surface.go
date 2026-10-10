package component

import "github.com/stubbedev/gelm/app"

type surfaceRef struct {
	window  *app.Window
	layer   *app.LayerWindow
	pending []func()
}

func (s *surfaceRef) attach(w *app.Window, l *app.LayerWindow) {
	s.window, s.layer = w, l
	pending := s.pending
	s.pending = nil
	for _, fn := range pending {
		fn()
	}
}

// Window returns the toplevel the component lives in: the one
// component.Window or Run opened for it or for an ancestor, nil
// otherwise and nil during Init (the window is created around the root
// Init returns).
func (cx *Context[In, Out]) Window() *app.Window { return cx.surface.window }

// Layer returns the layer surface the component lives in, like Window.
func (cx *Context[In, Out]) Layer() *app.LayerWindow { return cx.surface.layer }

// WatchWindow runs fn with the component's window as soon as it exists
// and again after every batch of updates: #[watch] on the window's own
// properties.
func (cx *Context[In, Out]) WatchWindow(fn func(*app.Window)) {
	run := func() {
		if w := cx.surface.window; w != nil {
			fn(w)
		}
	}
	cx.watches = append(cx.watches, run)
	if cx.surface.window == nil {
		cx.surface.pending = append(cx.surface.pending, run)
		return
	}
	run()
}

// WindowTitle keeps the window's title equal to get(), requesting a
// change only when the value moved.
func (cx *Context[In, Out]) WindowTitle(get func() string) {
	watchChanged(cx, get, (*app.Window).SetTitle)
}

// WindowMaximized keeps the window maximized while get() is true,
// requesting a change only when the value moved.
func (cx *Context[In, Out]) WindowMaximized(get func() bool) {
	watchChanged(cx, get, func(w *app.Window, on bool) {
		if on {
			w.Maximize()
		} else {
			w.Unmaximize()
		}
	})
}

// WindowFullscreen keeps the window fullscreen while get() is true,
// requesting a change only when the value moved.
func (cx *Context[In, Out]) WindowFullscreen(get func() bool) {
	watchChanged(cx, get, func(w *app.Window, on bool) {
		if on {
			w.Fullscreen()
		} else {
			w.Unfullscreen()
		}
	})
}

// WindowMinSize keeps the window's minimum size equal to get(),
// requesting a change only when the value moved.
func (cx *Context[In, Out]) WindowMinSize(get func() (width, height uint32)) {
	watchChanged(cx, func() [2]uint32 {
		w, h := get()
		return [2]uint32{w, h}
	}, func(w *app.Window, s [2]uint32) { w.SetMinSize(s[0], s[1]) })
}

func watchChanged[T comparable, In, Out any](cx *Context[In, Out], get func() T, apply func(*app.Window, T)) {
	var last T
	applied := false
	cx.WatchWindow(func(w *app.Window) {
		v := get()
		if applied && v == last {
			return
		}
		last, applied = v, true
		apply(w, v)
	})
}
