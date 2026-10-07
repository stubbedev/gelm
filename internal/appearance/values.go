package appearance

import (
	"github.com/godbus/dbus/v5"
)

// The tracked values and their portal mappings, #88 additions next to
// the existing ones.

// Accent is the desktop's accent-color preference: RGB components in
// [0,1], Known set when the desktop published one. The zero value is
// "no preference" - the same failure shape as Appearance's Unknown.
type Accent struct {
	R, G, B float64
	Known   bool
}

// Contrast is the desktop's contrast preference.
type Contrast uint8

// Contrast preferences.
const (
	// ContrastUnknown is no preference, no portal, or unreadable.
	ContrastUnknown Contrast = iota
	// ContrastHigh asks for the high-contrast presentation.
	ContrastHigh
)

// contrastHigh is the portal's one defined contrast value.
const contrastHigh uint32 = 1

// accentValue maps a portal value - a variant around a (ddd) RGB
// triple - to an Accent. Anything else (wrong arity, out-of-range
// components, wrong types) maps to the zero Accent rather than a
// guessed color.
func accentValue(v any) Accent {
	if variant, ok := v.(dbus.Variant); ok {
		v = variant.Value()
	}
	var rgb [3]float64
	switch triple := v.(type) {
	case []float64:
		if len(triple) != 3 {
			return Accent{}
		}
		rgb = [3]float64{triple[0], triple[1], triple[2]}
	case []any:
		if len(triple) != 3 {
			return Accent{}
		}
		for i, part := range triple {
			f, ok := part.(float64)
			if !ok {
				return Accent{}
			}
			rgb[i] = f
		}
	default:
		return Accent{}
	}
	for _, f := range rgb {
		if f < 0 || f > 1 {
			return Accent{}
		}
	}
	return Accent{R: rgb[0], G: rgb[1], B: rgb[2], Known: true}
}

// contrastValue maps a portal value - a variant around uint32 - to a
// Contrast: 1 is high contrast, everything else (0 "no preference",
// out of range, wrong type) is Unknown.
func contrastValue(v any) Contrast {
	if variant, ok := v.(dbus.Variant); ok {
		v = variant.Value()
	}
	if u, ok := v.(uint32); ok && u == contrastHigh {
		return ContrastHigh
	}
	return ContrastUnknown
}
