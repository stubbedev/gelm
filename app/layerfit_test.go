package app

import (
	"testing"

	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/widget"
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

// A toplevel without a requested size takes its content's natural
// extent on each zero axis; a requested axis stays as asked.
func TestDefaultSize(t *testing.T) {
	root := widget.NewDrawingArea(320, 180)
	if w, h := defaultSize(root, 0, 0, nil); w != 320 || h != 180 {
		t.Errorf("unsized window %dx%d, want the content's 320x180", w, h)
	}
	if w, h := defaultSize(root, 500, 0, nil); w != 500 || h != 180 {
		t.Errorf("width-only request %dx%d, want 500x180", w, h)
	}
	if w, h := defaultSize(root, 640, 480, nil); w != 640 || h != 480 {
		t.Errorf("sized window %dx%d, want 640x480 untouched", w, h)
	}
	if w, h := defaultSize(widget.NewDrawingArea(0, 0), 0, 0, nil); w != 1 || h != 1 {
		t.Errorf("empty content %dx%d, want the 1x1 floor", w, h)
	}
}
