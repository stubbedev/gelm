package widget

import (
	"fmt"

	"github.com/stubbedev/lys-ui/render"
)

// SurfaceHost is what a widget tree's surface does for its widgets:
// the application attaches one to every surface's root
// (SetSurfaceHost), and the widgets reach it through OpenMenu,
// OpenPopover and GrabFocus. The host owns the surface plumbing and
// reports its own failures.
type SurfaceHost interface {
	// OpenMenu opens items as a popup menu beside anchor, whose bounds
	// are in the tree's coordinates, painted with face at sizePx.
	OpenMenu(anchor Boundser, face render.Font, sizePx float64, items []MenuItem)
	// OpenPopover opens content in a popover beside anchor with focus
	// focused, and returns what closes it.
	OpenPopover(anchor Boundser, content, focus Widget) (close func())
	// Focus moves the surface's keyboard focus to w.
	Focus(w Widget)
}

// SetSurfaceHost makes h serve the tree under root. A root from
// outside this package carries no host, like SetStyleRoot's; asking
// its tree for a popup or a focus move then panics.
func SetSurfaceHost(root Widget, h SurfaceHost) {
	if n := nodeOf(root); n != nil {
		n.surfaceHost = h
	}
}

// hostOf is the host of w's tree; a tree no surface hosts is a caller
// bug, named by what asked.
func hostOf(w Widget, what string) SurfaceHost {
	for p := w; p != nil; p = parentOf(p) {
		if n := nodeOf(p); n != nil && n.surfaceHost != nil {
			return n.surfaceHost
		}
	}
	panic(fmt.Sprintf("widget: %s from %T: no surface hosts its tree", what, w))
}

// OpenMenu opens items as a popup menu beside anchor through the host
// of from's tree.
func OpenMenu(from Widget, anchor Boundser, face render.Font, sizePx float64, items []MenuItem) {
	hostOf(from, "OpenMenu").OpenMenu(anchor, face, sizePx, items)
}

// OpenPopover opens content in a popover beside anchor through the
// host of from's tree, with focus focused, and returns what closes it.
// The popover's own node is a box around content, so content keeps its
// element and style.
func OpenPopover(from Widget, anchor Boundser, content, focus Widget) (close func()) {
	return hostOf(from, "OpenPopover").OpenPopover(anchor, NewBox(Column, 0, 0).Append(content, true), focus)
}

// GrabFocus moves the keyboard focus of w's surface to w (GTK's
// gtk_widget_grab_focus): a form focusing its first invalid field. A
// widget that cannot take focus leaves it where it was.
func GrabFocus(w Widget) { hostOf(w, "GrabFocus").Focus(w) }
