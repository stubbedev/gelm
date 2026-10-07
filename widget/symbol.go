package widget

import (
	"math"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// SymbolKind names one of the vector glyphs the toolkit draws for its
// own chrome - disclosure arrows, sort indicators, window controls -
// instead of text glyphs, which fonts (the bundled fixture face among
// them) often lack and which then render as missing-glyph boxes.
type SymbolKind uint8

// Symbol kinds.
const (
	// SymbolChevronRight is a right-pointing chevron (collapsed, next).
	SymbolChevronRight SymbolKind = iota
	// SymbolChevronDown is a down-pointing chevron (expanded, descending).
	SymbolChevronDown
	// SymbolChevronLeft is a left-pointing chevron (back).
	SymbolChevronLeft
	// SymbolChevronUp is an up-pointing chevron (ascending).
	SymbolChevronUp
	// SymbolClose is a diagonal cross.
	SymbolClose
	// SymbolMinimize is a low horizontal bar.
	SymbolMinimize
	// SymbolMaximize is an outlined square.
	SymbolMaximize
	// SymbolCrosshair is a picker's crosshair.
	SymbolCrosshair
	// SymbolDoubleLeft is two left chevrons (a bigger step back).
	SymbolDoubleLeft
	// SymbolDoubleRight is two right chevrons (a bigger step forward).
	SymbolDoubleRight
)

// chevronAngle is each chevron kind's rotation from right-pointing.
var chevronAngle = map[SymbolKind]float64{
	SymbolChevronRight: 0,
	SymbolChevronDown:  math.Pi / 2,
	SymbolChevronLeft:  math.Pi,
	SymbolChevronUp:    -math.Pi / 2,
}

// Symbol paints one SymbolKind in a size x size box, stroked in the
// cascade's color over the theme's muted text. Styled as `image`.
type Symbol struct {
	node
	kind SymbolKind
	size int
}

// NewSymbol returns a symbol of kind in a size-pixel box.
func NewSymbol(kind SymbolKind, size int) *Symbol {
	s := &Symbol{kind: kind, size: size}
	s.SetElement("image")
	return s
}

// SetKind changes the glyph (a disclosure turning, a sort flipping).
func (s *Symbol) SetKind(kind SymbolKind) {
	if s.kind == kind {
		return
	}
	s.kind = kind
	s.Invalidate()
}

// Kind reports the glyph.
func (s *Symbol) Kind() SymbolKind { return s.kind }

// Measure wants the symbol's box.
func (s *Symbol) Measure(con Constraints) Size {
	return clampSize(Size{W: s.size, H: s.size}, con)
}

// Arrange records the rect.
func (s *Symbol) Arrange(r render.Rect) { s.node.Arrange(r) }

// Paint strokes the glyph centered in the bounds.
func (s *Symbol) Paint(cv *render.Canvas) {
	col := pickc(0, s.style(s), style.PropColor, Current().TextMuted)
	b := s.bounds
	cx, cy := b.X+b.W/2, b.Y+b.H/2
	r := min(b.W, b.H) / 3
	if ang, ok := chevronAngle[s.kind]; ok {
		strokeChevron(cv, cx, cy, r, ang, col)
		return
	}
	switch s.kind {
	case SymbolDoubleLeft, SymbolDoubleRight:
		ang := 0.0
		if s.kind == SymbolDoubleLeft {
			ang = math.Pi
		}
		strokeChevron(cv, cx-r/2, cy, r, ang, col)
		strokeChevron(cv, cx+r/2, cy, r, ang, col)
	case SymbolClose:
		cv.Line(cx-r, cy-r, cx+r, cy+r, 1, col)
		cv.Line(cx-r, cy+r, cx+r, cy-r, 1, col)
	case SymbolMinimize:
		cv.Line(cx-r, cy+r, cx+r, cy+r, 1, col)
	case SymbolMaximize:
		cv.Line(cx-r, cy-r, cx+r, cy-r, 1, col)
		cv.Line(cx+r, cy-r, cx+r, cy+r, 1, col)
		cv.Line(cx+r, cy+r, cx-r, cy+r, 1, col)
		cv.Line(cx-r, cy+r, cx-r, cy-r, 1, col)
	case SymbolCrosshair:
		cv.Line(cx-r, cy, cx+r, cy, 1, col)
		cv.Line(cx, cy-r, cx, cy+r, 1, col)
	}
}

// HitTest resolves inside the bounds.
func (s *Symbol) HitTest(p Point) Widget { return s.HitLeaf(s, p) }

// strokeChevron draws a v-shaped chevron centered on (cx, cy) with arm
// half-span r, rotated ang radians from right-pointing - the stroke the
// dropdown face and every Symbol chevron share.
func strokeChevron(cv *render.Canvas, cx, cy, r int, ang float64, col render.Color) {
	c, s := math.Cos(ang), math.Sin(ang)
	rot := func(px, py float64) (int, int) {
		return cx + int(math.Round(px*c-py*s)), cy + int(math.Round(px*s+py*c))
	}
	h := float64(r)
	tx, ty := rot(h/2, 0)
	ax, ay := rot(-h/2, -h)
	bx, by := rot(-h/2, h)
	cv.Line(ax, ay, tx, ty, 1, col)
	cv.Line(tx, ty, bx, by, 1, col)
}
