package style

import (
	"strings"
)

// CursorSpec is the cursor property's value: a CSS keyword, or a
// url() file cursor with its hotspot in pixels. The zero value is
// auto — the widget's own shape decides.
type CursorSpec struct {
	// Name is the keyword, or with a URL the fallback keyword the
	// list ends with; empty is auto.
	Name string
	// URL is the first url() image and HotX, HotY its hotspot.
	URL        string
	HotX, HotY int
}

// PointerEvents is the pointer-events keyword: none takes the widget
// (and its subtree) out of hit testing.
type PointerEvents uint8

// Pointer-events values.
const (
	PointerAuto PointerEvents = iota
	PointerNone
)

// UserSelect is the user-select keyword.
type UserSelect uint8

// User-select values. Auto's used value depends on the element and its
// parent (CSS UI 4 §7.1); contain selects like text, confined to the
// element.
const (
	SelectAuto UserSelect = iota
	SelectText
	SelectNone
	SelectAll
	SelectContain
)

// CaretShape is the caret-shape keyword.
type CaretShape uint8

// Caret shapes.
const (
	CaretAuto CaretShape = iota
	CaretBar
	CaretBlock
	CaretUnderscore
)

// Appearance is the appearance keyword.
type Appearance uint8

// Appearance values.
const (
	AppearanceAuto Appearance = iota
	AppearanceNone
)

// TouchAction is touch-action's pan permissions as an axis bitmask:
// TouchAuto allows both, the single-axis values one, TouchNone
// neither.
type TouchAction uint8

// Touch-action axis bits.
const (
	TouchPanX TouchAction = 1 << iota
	TouchPanY
)

// TouchAuto allows every pan axis; TouchNone allows none.
const (
	TouchAuto             = TouchPanX | TouchPanY
	TouchNone TouchAction = 0
)

// cursorKeywords are the cursor property's keywords, mapped onto the
// xcursor vocabulary the session resolves (the CSS spellings the
// cursor-shape protocol and the themes carry).
var cursorKeywords = map[string]bool{
	"auto": true, "default": true, "none": true,
	"context-menu": true, "help": true, "pointer": true,
	"progress": true, "wait": true, "cell": true, "crosshair": true,
	"text": true, "vertical-text": true, "alias": true, "copy": true,
	"move": true, "no-drop": true, "not-allowed": true,
	"grab": true, "grabbing": true,
	"e-resize": true, "n-resize": true, "ne-resize": true,
	"nw-resize": true, "s-resize": true, "se-resize": true,
	"sw-resize": true, "w-resize": true,
	"ew-resize": true, "ns-resize": true, "nesw-resize": true,
	"nwse-resize": true, "col-resize": true, "row-resize": true,
	"all-scroll": true, "zoom-in": true, "zoom-out": true,
}

// parseCursor parses cursor: the first comma entry the engine can
// honor — a url() file cursor (with an optional x y hotspot) or a
// keyword — wins, the rest are fallbacks it drops.
func parseCursor(ts []token, _ *ctx, v *Values) bool {
	entries := splitTop(ts, tkComma)
	var spec CursorSpec
	for i, e := range entries {
		comps := components(e)
		if len(comps) == 0 {
			return false
		}
		if comps[0][0].kind == tkFunc && comps[0][0].s == "url" {
			u, ok := backgroundImageURL(comps[0])
			if !ok {
				return false
			}
			var hx, hy int
			switch len(comps) {
			case 1:
			case 3:
				x, ok1 := intComponent(comps[1])
				y, ok2 := intComponent(comps[2])
				if !ok1 || !ok2 {
					return false
				}
				hx, hy = x, y
			default:
				return false
			}
			if spec.URL == "" {
				spec.URL, spec.HotX, spec.HotY = u, hx, hy
			}
			continue
		}
		if i != len(entries)-1 || len(comps) != 1 || comps[0][0].kind != tkIdent {
			return false
		}
		name := strings.ToLower(comps[0][0].s)
		if !cursorKeywords[name] {
			return false
		}
		if name != "auto" {
			spec.Name = name
		}
	}
	v.Cursor = spec
	return true
}

// intComponent reads one non-negative integer component.
func intComponent(ts []token) (int, bool) {
	if len(ts) != 1 || ts[0].kind != tkNumber || ts[0].num < 0 {
		return 0, false
	}
	return int(ts[0].num), true
}

// parsePointerEvents parses pointer-events: auto or none.
func parsePointerEvents(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "auto":
		v.PointerEvents = PointerAuto
	case "none":
		v.PointerEvents = PointerNone
	default:
		return false
	}
	return true
}

// parseUserSelect parses user-select: none, text, all, or auto (as
// text).
func parseUserSelect(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "auto":
		v.UserSelect = SelectAuto
	case "text":
		v.UserSelect = SelectText
	case "contain":
		v.UserSelect = SelectContain
	case "none":
		v.UserSelect = SelectNone
	case "all":
		v.UserSelect = SelectAll
	default:
		return false
	}
	return true
}

// parseCaretShape parses caret-shape: auto, bar, block, or
// underscore.
func parseCaretShape(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "auto":
		v.CaretShape = CaretAuto
	case "bar":
		v.CaretShape = CaretBar
	case "block":
		v.CaretShape = CaretBlock
	case "underscore":
		v.CaretShape = CaretUnderscore
	default:
		return false
	}
	return true
}

// parseAccentColor parses accent-color: auto or a color.
func parseAccentColor(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) == 1 && first[0].ident("auto") {
		v.AccentAuto = true
		v.AccentColor = 0
		return true
	}
	cv, ok := parseColor(first, cx)
	if !ok {
		return false
	}
	v.AccentAuto = false
	v.AccentColor = cx.resolve(cv)
	return true
}

// parseAppearance parses appearance: none or auto.
func parseAppearance(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "auto":
		v.Appearance = AppearanceAuto
	case "none":
		v.Appearance = AppearanceNone
	default:
		return false
	}
	return true
}

// touchActionKeywords maps each touch-action keyword onto the axes it
// permits. pan-left/right/up/down are direction-restricted pans the
// scroll gates cannot tell from their axis, so they permit the axis
// (the stricter reading would block both). manipulation is pan-x
// pan-y with zooming disabled; the pinch deferral makes it every pan
// axis here.
var touchActionKeywords = map[string]TouchAction{
	"auto": TouchAuto, "none": TouchNone,
	"pan-x": TouchPanX, "pan-left": TouchPanX, "pan-right": TouchPanX,
	"pan-y": TouchPanY, "pan-up": TouchPanY, "pan-down": TouchPanY,
	"manipulation": TouchAuto,
}

// parseTouchAction parses touch-action: one keyword or a
// space-separated pan combination.
func parseTouchAction(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 {
		return false
	}
	var act TouchAction
	for _, c := range comps {
		if len(c) != 1 || c[0].kind != tkIdent {
			return false
		}
		a, ok := touchActionKeywords[strings.ToLower(c[0].s)]
		if !ok {
			return false
		}
		// auto and none stand alone; pans combine with pans.
		if len(comps) > 1 && a == TouchAuto {
			return false
		}
		act |= a
	}
	v.TouchAction = act
	return true
}
