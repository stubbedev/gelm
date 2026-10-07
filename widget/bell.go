package widget

import "sync/atomic"

// errorBell is the installed bell (app rings the widget's window
// through xdg-system-bell); nil makes ErrorBell silent.
var errorBell atomic.Pointer[func(Widget)]

// SetErrorBell installs the bell ErrorBell rings; nil silences it.
func SetErrorBell(fn func(w Widget)) {
	if fn == nil {
		errorBell.Store(nil)
		return
	}
	errorBell.Store(&fn)
}

// ErrorBell signals an input w rejected (gtk_widget_error_bell):
// typing into a read-only field, a value that does not parse. The
// application rings the system bell for w's window; the compositor may
// flash it instead.
func ErrorBell(w Widget) {
	if fn := errorBell.Load(); fn != nil {
		(*fn)(w)
	}
}

// RootOf is the top of w's tree: the widget with no parent above it.
func RootOf(w Widget) Widget {
	for {
		p := parentOf(w)
		if p == nil {
			return w
		}
		w = p
	}
}
