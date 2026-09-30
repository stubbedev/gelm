package style

import (
	"math"

	"github.com/stubbedev/gelm/render"
)

// numKind is the type of a numeric value after unit resolution.
type numKind uint8

const (
	numNumber  numKind = iota + 1 // unitless
	numLength                     // pixels
	numPercent                    // percent (not resolved against anything)
	numAngle                      // degrees
	numTime                       // seconds
)

// numeric is a typed number: lengths in px, angles in degrees, times in
// seconds.
type numeric struct {
	v    float64
	kind numKind
}

// ctx is the unit-resolution context of one node's compute: the rem
// base, the font size em resolves against, the inherited weight bolder
// and lighter step from, and the node's color for currentColor. dep
// records whether a parse read any node-dependent input, so a value
// that read none can be cached on its declaration.
type ctx struct {
	rem    float64
	em     float64
	weight int
	color  render.Color
	dep    bool
	// usedRem records a rem read: a cached value is only valid under
	// the base it was computed with.
	usedRem bool
}

// emPx returns the em base, noting the node dependence.
func (cx *ctx) emPx() float64 {
	cx.dep = true
	return cx.em
}

// wt returns the inherited font weight, noting the node dependence.
func (cx *ctx) wt() int {
	cx.dep = true
	return cx.weight
}

// cur returns currentColor, noting the node dependence.
func (cx *ctx) cur() render.Color {
	cx.dep = true
	return cx.color
}

// resolve turns a parsed color into a concrete one.
func (cx *ctx) resolve(c colorVal) render.Color {
	if c.current {
		return cx.cur()
	}
	return c.c
}

// unitScale converts a dimension unit to its canonical kind and factor.
func (cx *ctx) unitScale(unit string) (numKind, float64, bool) {
	switch unit {
	case "px":
		return numLength, 1, true
	case "rem":
		cx.usedRem = true
		return numLength, cx.rem, true
	case "em":
		return numLength, cx.emPx(), true
	case "pt":
		return numLength, 96.0 / 72, true
	case "pc":
		return numLength, 16, true
	case "in":
		return numLength, 96, true
	case "cm":
		return numLength, 96 / 2.54, true
	case "mm":
		return numLength, 96 / 25.4, true
	case "deg":
		return numAngle, 1, true
	case "rad":
		return numAngle, 180 / math.Pi, true
	case "grad":
		return numAngle, 0.9, true
	case "turn":
		return numAngle, 360, true
	case "s":
		return numTime, 1, true
	case "ms":
		return numTime, 0.001, true
	}
	return 0, 0, false
}

// evalNumeric evaluates one component value — a number, percentage,
// dimension, or calc()/min()/max()/clamp() expression — to a typed
// number. The tokens must be exactly one component.
func evalNumeric(ts []token, cx *ctx) (numeric, bool) {
	ts = trimWS(ts)
	if len(ts) == 0 {
		return numeric{}, false
	}
	e := calcParser{ts: ts, cx: cx}
	n, ok := e.term()
	if !ok || e.pos != len(ts) {
		return numeric{}, false
	}
	return n, true
}

// calcParser evaluates calc() expressions over tokens: sums, products,
// parentheses, and nested math functions, with CSS's type rules
// (lengths add to lengths, a product needs a unitless side, a quotient
// a unitless divisor).
type calcParser struct {
	ts  []token
	pos int
	cx  *ctx
}

func (p *calcParser) skipWS() {
	for p.pos < len(p.ts) && p.ts[p.pos].kind == tkWS {
		p.pos++
	}
}

// term parses one leaf: a literal or a math function.
func (p *calcParser) term() (numeric, bool) {
	if p.pos >= len(p.ts) {
		return numeric{}, false
	}
	t := p.ts[p.pos]
	switch t.kind {
	case tkNumber:
		p.pos++
		return numeric{v: t.num, kind: numNumber}, true
	case tkPercent:
		p.pos++
		return numeric{v: t.num, kind: numPercent}, true
	case tkDim:
		p.pos++
		kind, f, ok := p.cx.unitScale(t.s)
		if !ok {
			return numeric{}, false
		}
		return numeric{v: t.num * f, kind: kind}, true
	case tkLParen:
		end := blockEnd(p.ts, p.pos)
		sub := calcParser{ts: p.ts[p.pos+1 : end-1], cx: p.cx}
		n, ok := sub.sum()
		if !ok || sub.pos != len(sub.ts) {
			return numeric{}, false
		}
		p.pos = end
		return n, true
	case tkFunc:
		end := blockEnd(p.ts, p.pos)
		args := funcArgs(p.ts[p.pos:end])
		p.pos = end
		return mathFunc(t.s, args, p.cx)
	}
	return numeric{}, false
}

// mathFunc evaluates calc(), min(), max(), and clamp().
func mathFunc(name string, args []token, cx *ctx) (numeric, bool) {
	switch name {
	case "calc":
		sub := calcParser{ts: args, cx: cx}
		sub.skipWS()
		n, ok := sub.sum()
		sub.skipWS()
		return n, ok && sub.pos == len(args)
	case "min", "max", "clamp":
		var vals []numeric
		for _, a := range splitTop(args, tkComma) {
			sub := calcParser{ts: a, cx: cx}
			sub.skipWS()
			n, ok := sub.sum()
			sub.skipWS()
			if !ok || sub.pos != len(a) || len(vals) > 0 && vals[0].kind != n.kind {
				return numeric{}, false
			}
			vals = append(vals, n)
		}
		if len(vals) == 0 || name == "clamp" && len(vals) != 3 {
			return numeric{}, false
		}
		out := vals[0]
		switch name {
		case "min":
			for _, v := range vals[1:] {
				out.v = math.Min(out.v, v.v)
			}
		case "max":
			for _, v := range vals[1:] {
				out.v = math.Max(out.v, v.v)
			}
		case "clamp":
			out.v = math.Max(vals[0].v, math.Min(vals[1].v, vals[2].v))
		}
		return out, true
	}
	return numeric{}, false
}

// sum parses product (('+'|'-') product)*. CSS requires whitespace
// around + and -; a signed number token after whitespace (`a -1px`) is
// read as a subtraction too, since that is what the source means.
func (p *calcParser) sum() (numeric, bool) {
	left, ok := p.product()
	if !ok {
		return numeric{}, false
	}
	for {
		save := p.pos
		p.skipWS()
		if p.pos >= len(p.ts) {
			return left, true
		}
		t := p.ts[p.pos]
		var sign float64
		switch {
		case t.is('+'):
			sign = 1
			p.pos++
		case t.is('-'):
			sign = -1
			p.pos++
		case (t.kind == tkNumber || t.kind == tkDim || t.kind == tkPercent) && t.num < 0 && p.pos > save:
			// `x -2px`: the sign lexed into the number.
			sign = 1
		default:
			p.pos = save
			return left, true
		}
		p.skipWS()
		right, ok := p.product()
		if !ok || right.kind != left.kind {
			// A unitless zero adds to anything, as in `calc(0 + 1px)`.
			if ok && right.v == 0 && right.kind == numNumber {
				right.kind = left.kind
			} else if ok && left.v == 0 && left.kind == numNumber {
				left.kind = right.kind
			} else {
				return numeric{}, false
			}
		}
		left.v += sign * right.v
	}
}

// product parses term (('*'|'/') term)*.
func (p *calcParser) product() (numeric, bool) {
	left, ok := p.term()
	if !ok {
		return numeric{}, false
	}
	for {
		save := p.pos
		p.skipWS()
		if p.pos >= len(p.ts) {
			return left, true
		}
		t := p.ts[p.pos]
		if !t.is('*') && !t.is('/') {
			p.pos = save
			return left, true
		}
		p.pos++
		p.skipWS()
		right, ok := p.term()
		if !ok {
			return numeric{}, false
		}
		if t.is('*') {
			switch {
			case right.kind == numNumber:
				left.v *= right.v
			case left.kind == numNumber:
				left = numeric{v: left.v * right.v, kind: right.kind}
			default:
				return numeric{}, false
			}
			continue
		}
		if right.kind != numNumber || right.v == 0 {
			return numeric{}, false
		}
		left.v /= right.v
	}
}

// lengthOf evaluates a length component: a dimension, a unitless zero,
// or a math function yielding a length. Unitless non-zero numbers are
// accepted as pixels too — GTK stylesheets in the wild write them, and
// the pre-var engine took them.
func lengthOf(ts []token, cx *ctx) (float64, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return 0, false
	}
	switch n.kind {
	case numLength, numNumber:
		return n.v, true
	}
	return 0, false
}

// px rounds a length to whole logical pixels, halves away from zero.
func px(v float64) int { return int(math.Round(v)) }
