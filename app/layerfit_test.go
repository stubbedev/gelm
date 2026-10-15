package app

import (
	"testing"

	"github.com/stubbedev/gelm/internal/layersurface"
)

// Only an automatic axis missing one of its anchors follows the
// content; a concrete axis, or one anchored on both edges, does not.
func TestContentAxes(t *testing.T) {
	for name, tc := range map[string]struct {
		w, h   uint32
		anchor layersurface.Anchor
		want   fitAxes
	}{
		"one edge, both auto":       {0, 0, layersurface.AnchorBottom, fitAxes{w: true, h: true}},
		"corner, both auto":         {0, 0, layersurface.AnchorTop | layersurface.AnchorRight, fitAxes{w: true, h: true}},
		"bar: stretched width":      {0, 30, layersurface.AnchorTop | layersurface.AnchorLeft | layersurface.AnchorRight, fitAxes{}},
		"fixed width, auto height":  {400, 0, layersurface.AnchorTop, fitAxes{h: true}},
		"full screen":               {0, 0, layersurface.AnchorTop | layersurface.AnchorBottom | layersurface.AnchorLeft | layersurface.AnchorRight, fitAxes{}},
		"side panel: stretched h":   {0, 0, layersurface.AnchorLeft | layersurface.AnchorTop | layersurface.AnchorBottom, fitAxes{w: true}},
		"concrete on a single edge": {120, 40, layersurface.AnchorBottom, fitAxes{}},
	} {
		if got := contentAxes(tc.w, tc.h, tc.anchor); got != tc.want {
			t.Errorf("%s: %+v, want %+v", name, got, tc.want)
		}
	}
}
