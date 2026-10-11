// The CSS interaction surfaces (docs/css.md): cursor, pointer-events,
// user-select, caret-shape, accent-color, appearance, and touch-action
// — the computed values the input path and the painters read.
package widget

import (
	"strconv"
	"strings"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// cursorFilePrefix marks a cursor request that asks for a file cursor
// (CSS cursor: url()): `file:<path>#<x>,<y>`, the hotspot CSS named.
const cursorFilePrefix = "file:"

// CSSCursorOf returns the cursor the stylesheet asks for over w: an
// xcursor shape name the session resolves ("pointer", "ns-resize",
// the hidden "none"), or a file request built from cursor: url() and
// its hotspot. Empty when no cursor declaration reaches w or it is
// auto — the caller falls back to the widget's own shape. The value
// inherits, so one declaration covers a subtree.
func CSSCursorOf(w Widget) string {
	n := nodeOf(w)
	if n == nil {
		return ""
	}
	return cssCursorRequest(n.style(w).Cursor)
}

// cssCursorRequest maps the computed cursor onto the request the app
// hands the session: keywords pass through by their CSS spelling, a
// url() becomes a file request, auto asks for nothing.
func cssCursorRequest(spec style.CursorSpec) string {
	if spec.URL != "" {
		return cursorFilePrefix + spec.URL + "#" +
			strconv.Itoa(spec.HotX) + "," + strconv.Itoa(spec.HotY)
	}
	return spec.Name
}

// pointerEventsNoneWithin reports whether w or any ancestor declared
// pointer-events: none — the subtree is transparent to hit testing,
// and a press falls through to whatever is behind it.
func pointerEventsNoneWithin(w Widget) bool {
	for ; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil {
			if v := n.style(w); v.Has(style.PropPointerEvents) && v.PointerEvents == style.PointerNone {
				return true
			}
		}
	}
	return false
}

// userSelectOf returns the user-select value that reaches w, text
// when nothing declares one.
func userSelectOf(w Widget) style.UserSelect {
	n := nodeOf(w)
	if n == nil {
		return style.SelectText
	}
	if v := n.style(w); v.Has(style.PropUserSelect) {
		return v.UserSelect
	}
	return style.SelectText
}

// accentOf resolves accent-color over the fallback color: the
// sheet's color when it declares one, else def (the theme accent or
// its disabled derivation).
func accentOf(v *style.Values, def render.Color) render.Color {
	if v.Has(style.PropAccentColor) && !v.AccentAuto {
		return v.AccentColor
	}
	return def
}

// touchActionOf intersects the touch-action declarations from w up
// the chain into the pan axes a touch scroll may still move.
func touchActionOf(w Widget) style.TouchAction {
	act := style.TouchAuto
	for ; w != nil; w = parentOf(w) {
		n := nodeOf(w)
		if n == nil {
			continue
		}
		if v := n.style(w); v.Has(style.PropTouchAction) {
			act &= v.TouchAction
		}
	}
	return act
}

// gateTouchAxis masks scroll deltas by the touch-action that reaches
// w: none stops both axes, pan-x and pan-y keep theirs. The wheel
// path is not gated — touch-action speaks to touch manipulation
// only.
func gateTouchAxis(w Widget, dx, dy *float64) {
	switch touchActionOf(w) {
	case style.TouchNone:
		*dx, *dy = 0, 0
	case style.TouchPanX:
		*dy = 0
	case style.TouchPanY:
		*dx = 0
	}
}

// caretWidth is the block and underscore caret's fallback width when
// the caret sits past the text's end.
const caretWidth = 8

// caretShapeRect is the caret's rect in its CSS shape: the 2px bar,
// the block as wide as the cluster past the caret, the underscore
// along the text area's bottom. adv is that cluster's width, 0 past
// the end.
func caretShapeRect(x, y, h, adv int, shape style.CaretShape) render.Rect {
	switch shape {
	case style.CaretBlock:
		return render.Rect{X: x, Y: y, W: max(adv, 2), H: h}
	case style.CaretUnderscore:
		return render.Rect{X: x, Y: y + h - 2, W: max(adv, caretWidth), H: 2}
	default:
		return render.Rect{X: x, Y: y, W: 2, H: h}
	}
}

// fileCursorRequest splits a file cursor request back into its path
// and hotspot; ok reports it is one.
func fileCursorRequest(name string) (path string, hx, hy int, ok bool) {
	if !strings.HasPrefix(name, cursorFilePrefix) {
		return "", 0, 0, false
	}
	rest := strings.TrimPrefix(name, cursorFilePrefix)
	path, hot, found := strings.Cut(rest, "#")
	if !found || path == "" {
		return "", 0, 0, false
	}
	xy := strings.Split(hot, ",")
	if len(xy) != 2 {
		return "", 0, 0, false
	}
	var err error
	if hx, err = strconv.Atoi(xy[0]); err != nil {
		return "", 0, 0, false
	}
	if hy, err = strconv.Atoi(xy[1]); err != nil {
		return "", 0, 0, false
	}
	return path, hx, hy, true
}
