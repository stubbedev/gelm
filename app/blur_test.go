package app

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The compositor blur region's wire traffic (#196): nothing unless the
// window asks; the root's rounded shape once, again only when the
// shape changes, and a removal when the window stops asking.
func TestBlurRegionFollowsTheRootShape(t *testing.T) {
	root := widget.NewBox(widget.Column, 0, 0)
	root.SetInlineStyle("border-radius: 12px;")
	h := newPaintHarness(root, 120, 80)
	h.surf.blurOffered = true

	h.frame()
	if len(h.surf.blur) != 0 {
		t.Fatalf("a window that never asked sent %d blur regions", len(h.surf.blur))
	}

	h.wnd.cfg.blur = true
	h.frame()
	want := render.RoundedRectRegion(render.Rect{W: 120, H: 80}, render.UniformCorners(12))
	if len(h.surf.blur) != 1 || !slices.Equal(h.surf.blur[0], want) {
		t.Fatalf("blur regions %v, want the rounded root shape once", h.surf.blur)
	}
	if !h.wnd.blurActive() {
		t.Error("blur not reported active after its region went out")
	}

	h.frame()
	if len(h.surf.blur) != 1 {
		t.Errorf("an unchanged frame re-sent the region (%d sends)", len(h.surf.blur))
	}

	h.wnd.host.(*fakeHost).resizeTo(200, 100)
	h.frame()
	want = render.RoundedRectRegion(render.Rect{W: 200, H: 100}, render.UniformCorners(12))
	if len(h.surf.blur) != 2 || !slices.Equal(h.surf.blur[1], want) {
		t.Errorf("after the resize: %v, want the new shape", h.surf.blur)
	}

	h.wnd.cfg.blur = false
	h.frame()
	h.frame()
	if len(h.surf.blur) != 3 || h.surf.blur[2] != nil || h.wnd.blurActive() {
		t.Errorf("turning blur off sent %v, want exactly one removal", h.surf.blur)
	}
}

// Asked for while the compositor offers no blur, nothing goes on the
// wire and the root carries css.BlurUnavailable, so .glass turns
// opaque; the offer arriving sends the region and lifts the class.
func TestBlurFallbackClass(t *testing.T) {
	root := widget.NewBox(widget.Column, 0, 0)
	root.AddClass(css.Glass)
	h := newPaintHarness(root, 80, 60)
	h.wnd.cfg.blur = true

	h.frame()
	if len(h.surf.blur) != 0 || !widget.HasClass(root, css.BlurUnavailable) {
		t.Fatalf("unoffered blur: %d sends, fallback class %v", len(h.surf.blur), widget.HasClass(root, css.BlurUnavailable))
	}

	h.surf.blurOffered = true
	h.frame()
	if len(h.surf.blur) != 1 || widget.HasClass(root, css.BlurUnavailable) {
		t.Errorf("offered blur: %d sends, fallback class %v; want the region and no class", len(h.surf.blur), widget.HasClass(root, css.BlurUnavailable))
	}
}
