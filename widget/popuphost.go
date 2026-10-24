package widget

import (
	"fmt"

	"github.com/stubbedev/lys-ui/render"
)

// PopupHost opens popups for a widget tree: the application attaches
// one to every surface's root (SetPopupHost), and the widgets in the
// tree reach it through OpenMenu. The host owns the surface plumbing
// and reports its own failures.
type PopupHost interface {
	// OpenMenu opens items as a popup menu beside anchor, whose bounds
	// are in the tree's coordinates, painted with face at sizePx.
	OpenMenu(anchor Boundser, face render.Font, sizePx float64, items []MenuItem)
}

// SetPopupHost makes h open the popups of the tree under root. A root
// from outside this package carries no host, like SetStyleRoot's; a
// popup opened in its tree then panics in OpenMenu.
func SetPopupHost(root Widget, h PopupHost) {
	if n := nodeOf(root); n != nil {
		n.popupHost = h
	}
}

// OpenMenu opens items as a popup menu beside anchor through the
// popup host of from's tree. A tree no surface hosts cannot open one:
// that is a caller bug and panics.
func OpenMenu(from Widget, anchor Boundser, face render.Font, sizePx float64, items []MenuItem) {
	for w := from; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil && n.popupHost != nil {
			n.popupHost.OpenMenu(anchor, face, sizePx, items)
			return
		}
	}
	panic(fmt.Sprintf("widget: OpenMenu from %T: no popup host above it", from))
}
