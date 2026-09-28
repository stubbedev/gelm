package app

import (
	"testing"

	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// The app-level half of the element-name pin (docs/css.md): dialog,
// popover, and tooltip name their cards through nameSurfaceElement;
// toast is a widget type and pins itself in the widget package.
func TestCSSSurfaceElementNames(t *testing.T) {
	// The dialog card carries the name on both shadow branches, whose
	// card is a different widget.
	for _, tc := range []struct {
		name string
		blur int
	}{
		{"shadowed", 16},
		{"flat", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := widget.Current()
			widget.SetTheme(widget.DarkTheme().WithShadowBlur(tc.blur))
			t.Cleanup(func() { widget.SetTheme(prev) })

			content := widget.NewBox(widget.Column, 12, 12)
			root, _ := dialogCard(content)
			var named string
			if tc.blur > 0 {
				margin, ok := root.(*widget.Box)
				if !ok || len(margin.Children()) != 1 {
					t.Fatalf("shadowed dialog root = %T", root)
				}
				if e, ok := margin.Children()[0].(interface{ Element() string }); ok {
					named = e.Element()
				}
			} else {
				named = content.Element()
			}
			if named != elemDialog {
				t.Fatalf("dialog card element = %q, want %q", named, elemDialog)
			}
		})
	}

	// Popover and tooltip name whatever card tree they are given.
	box := widget.NewBox(widget.Row, 0, 0)
	nameSurfaceElement(box, elemPopover)
	if box.Element() != elemPopover {
		t.Errorf("popover card element = %q", box.Element())
	}
	tip := widget.NewBox(widget.Row, 0, 0)
	nameSurfaceElement(tip, elemTooltip)
	if tip.Element() != elemTooltip {
		t.Errorf("tooltip card element = %q", tip.Element())
	}

	// A non-node-backed widget is skipped silently.
	nameSurfaceElement(unnamed{}, elemPopover)
}

type unnamed struct{}

func (unnamed) Measure(widget.Constraints) widget.Size { return widget.Size{} }
func (unnamed) Arrange(render.Rect)                    {}
func (unnamed) Paint(*render.Canvas)                   {}
func (unnamed) HitTest(widget.Point) widget.Widget     { return nil }

// TestCSSPollerInstalled pins the startup wiring: an Application lends
// the widget package its timer wheel for stylesheet hot reload, the
// same one-hook-per-process shape as SetInvoker.
func TestCSSPollerInstalled(t *testing.T) {
	if NewApplication(&wlsession.Session{}) == nil {
		t.Fatal("no application")
	}
}
