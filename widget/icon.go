package widget

import (
	"os"
	"path/filepath"

	"github.com/stubbedev/gelm/internal/icons"
	"github.com/stubbedev/gelm/render"
)

// iconKind says where an icon's pixels come from.
type iconKind uint8

const (
	iconStatic iconKind = iota // rasterized up front by the caller
	iconTheme                  // looked up in the icon theme by name
	iconFile                   // loaded from a file path
	iconSVG                    // rasterized from embedded SVG bytes
)

// Icon paints an icon at its natural size. The pixels come from one of
// four sources:
//
//   - NewIcon: a raster the caller already made (render.LoadSVG and
//     friends); the widget draws it as is.
//   - NewThemeIcon: an icon theme name, resolved through the shared
//     icons.Cache at the canvas device scale and re-resolved when the
//     theme or the accent moves.
//   - NewFileIcon: an image file (png, svg, svgz).
//   - NewSVGIcon: embedded SVG bytes, for apps that ship their own
//     glyphs.
//
// Symbolic sources - -symbolic theme files and SVGs painting with
// currentColor - recolor to the theme Accent so glyphs follow the
// palette; SetTint pins a different color. The recoloring is a mask
// pass (render.Icon.Tint): alpha is kept, ink takes the tint, so
// gradients collapse to their alpha ramp - the symbolic contract.
type Icon struct {
	node
	ic *render.Icon

	kind iconKind
	name string // theme icon name
	path string // file path
	svg  []byte // embedded SVG bytes
	size int    // logical box for the non-static kinds

	// tint pins the recolor color; zero follows the theme Accent.
	tint Color

	// resolution state: the tint and 120-based device scale the
	// current raster was made for, and the icons.Cache generation it
	// was resolved at (theme kind only). err holds the last resolution
	// failure; it sticks until a generation bump so a missing icon
	// costs one lookup, not one per frame.
	resolvedTint Color
	resolvedFrac uint32
	resolvedGen  uint64
	err          error
}

// NewIcon returns an icon widget around a rasterized icon.
func NewIcon(ic *render.Icon) *Icon {
	return &Icon{ic: ic}
}

// NewThemeIcon returns an icon widget that resolves name from the icon
// theme at size logical pixels, rasterized at the canvas device scale.
// Symbolic icons recolor to the theme Accent. Resolution failures paint
// nothing and surface through Err.
func NewThemeIcon(name string, size int) *Icon {
	return &Icon{kind: iconTheme, name: name, size: size}
}

// NewFileIcon returns an icon widget that loads an image file (png,
// svg, svgz) and draws it at size logical pixels.
func NewFileIcon(path string, size int) *Icon {
	return &Icon{kind: iconFile, path: path, size: size}
}

// NewSVGIcon returns an icon widget that rasterizes embedded SVG bytes
// at size logical pixels.
func NewSVGIcon(svg []byte, size int) *Icon {
	return &Icon{kind: iconSVG, svg: svg, size: size}
}

// SetTint pins the symbolic recolor color; the zero color (the default)
// follows the theme Accent. On a static icon the raster is already
// final, so the tint is applied once, immediately, and cannot be
// undone.
func (i *Icon) SetTint(tint Color) {
	if tint == i.tint {
		return
	}
	i.tint = tint
	if i.kind == iconStatic {
		if i.ic != nil && tint != 0 {
			i.ic = i.ic.Tint(tint)
		}
		return
	}
	i.expire()
}

// Tint returns the pinned recolor color; the zero color (the default)
// follows the theme Accent.
func (i *Icon) Tint() Color { return i.tint }

// Name returns the theme icon's name; empty for the other kinds.
func (i *Icon) Name() string { return i.name }

// SetThemeName swaps a theme-resolved icon's name - the state-icon
// pattern (battery, volume, network): one widget, the glyph follows
// the state. A non-theme icon ignores the call; the next paint
// rasterizes the new glyph.
func (i *Icon) SetThemeName(name string) {
	if i.kind != iconTheme || i.name == name {
		return
	}
	i.name = name
	i.err = nil
	i.expire()
}

// Err returns the resolution failure of a dynamic icon: an unknown
// theme name, a missing file, or undecodable data. Nil until a paint
// tried and failed.
func (i *Icon) Err() error { return i.err }

// Measure returns the icon's pixel size, clamped to con.
func (i *Icon) Measure(con Constraints) Size {
	if sz, ok := i.measureHit(con); ok {
		return sz
	}
	w, h := i.size, i.size
	if i.kind == iconStatic && i.ic != nil {
		w, h = i.ic.Size()
	}
	return i.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Paint draws the icon at the top-left of the arranged rect. Dynamic
// icons resolve first, re-rasterizing when the canvas device scale, the
// tint, or the icon theme cache moved on since the last frame.
func (i *Icon) Paint(cv *render.Canvas) {
	if i.kind != iconStatic {
		i.resolveFor(cv)
		if i.ic == nil {
			return
		}
	}
	i.ic.Draw(cv, i.bounds.X, i.bounds.Y)
}

// HitTest returns the icon when p is inside its bounds.
func (i *Icon) HitTest(p Point) Widget {
	return i.HitLeaf(i, p)
}

// takeDamage drains the icon's pending repaint. A themed icon first
// expires its raster when the icons.Cache generation moved on (a
// SetTheme or InvalidateTheme call - there is no live theme-change
// signal by design; see icons.Cache), so the repaint this pass owes
// draws the new theme. Without the check the damage collector would
// see a clean widget and skip the switch.
func (i *Icon) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	if i.kind == iconTheme && i.resolvedGen != icons.Default().Generation() {
		i.expire()
	}
	return i.node.takeDamage()
}

// expire drops the resolved raster so the next paint re-resolves, and
// schedules a repaint.
func (i *Icon) expire() {
	i.ic = nil
	i.err = nil
	i.resolvedTint = 0
	i.resolvedFrac = 0
	i.resolvedGen = 0
	i.Invalidate()
}

// resolveFor re-resolves the source when the last raster no longer
// matches this canvas.
func (i *Icon) resolveFor(cv *render.Canvas) {
	num, denom := cv.DeviceScale()
	frac := fracOf(num, denom)
	tint := i.tint
	if tint == 0 {
		tint = Current().Accent
	}
	if !i.stale(frac, tint) {
		return
	}
	i.resolve(frac, tint)
}

// stale reports whether the resolved raster predates the canvas's
// device scale, the effective tint, or the icon theme cache generation.
func (i *Icon) stale(frac uint32, tint Color) bool {
	if i.kind == iconTheme && i.resolvedGen != icons.Default().Generation() {
		return true
	}
	if i.err != nil {
		return false
	}
	if i.ic == nil {
		return true
	}
	return i.resolvedFrac != frac || i.resolvedTint != tint
}

// resolve rasterizes the source into a device box of
// icons.DeviceBox(size, frac) pixels and records what it was made for.
func (i *Icon) resolve(frac uint32, tint Color) {
	dev := icons.DeviceBox(i.size, frac)
	ic, err := i.rasterize(frac, tint, dev)
	i.resolvedFrac, i.resolvedTint = frac, tint
	if i.kind == iconTheme {
		i.resolvedGen = icons.Default().Generation()
	}
	if err != nil {
		i.err, i.ic = err, nil
		return
	}
	i.err, i.ic = nil, ic
}

// rasterize decodes the source at dev pixels, mask-recoloring it to
// tint when it is symbolic or a tint is pinned.
func (i *Icon) rasterize(frac uint32, tint Color, dev int) (*render.Icon, error) {
	switch i.kind {
	case iconTheme:
		cache := icons.Default()
		if i.tint != 0 {
			return cache.Tinted(i.name, i.size, frac, tint)
		}
		return cache.SymbolicIcon(i.name, i.size, frac, tint)
	case iconFile:
		data, err := os.ReadFile(i.path)
		if err != nil {
			return nil, err
		}
		return tintedImage(data, filepath.Base(i.path), dev, tint, i.tint != 0 || icons.IsSymbolic(filepath.Base(i.path), data))
	default:
		return tintedImage(i.svg, "icon.svg", dev, tint, i.tint != 0 || icons.IsSymbolic("icon.svg", i.svg))
	}
}

// tintedImage decodes data into a dev x dev box and, when mask is set,
// recolors it: every pixel keeps its alpha and takes tint.
func tintedImage(data []byte, name string, dev int, tint Color, mask bool) (*render.Icon, error) {
	ic, err := icons.DecodeImage(data, name, dev, dev)
	if err != nil {
		return nil, err
	}
	if mask {
		ic = ic.Tint(tint)
	}
	return ic, nil
}

// fracOf converts a canvas device scale (num device pixels per denom
// logical pixels) to the 120-based convention of internal/scale: 120 is
// 1x, 240 is 2x. The app pipeline always feeds canvases a denominator
// of 120, so the conversion is exact there.
func fracOf(num, denom int) uint32 {
	if num <= 0 || denom <= 0 {
		return 0
	}
	return uint32((num*120 + denom/2) / denom)
}
