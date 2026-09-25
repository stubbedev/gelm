package app

import (
	"testing"

	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// fakeTranslator stands in for the session's keymap translation.
type fakeTranslator struct {
	text map[uint32]string
	syms map[uint32]xkb.Keysym
}

// KeyUTF8 implements keyTranslator.
func (f *fakeTranslator) KeyUTF8(code uint32) string { return f.text[code] }

// KeySym implements keyTranslator.
func (f *fakeTranslator) KeySym(code uint32) xkb.Keysym { return f.syms[code] }

func TestRouteKey(t *testing.T) {
	newRouter := func() (*widget.Router, *widget.Entry) {
		face, err := render.LoadFont(goregular.TTF)
		if err != nil {
			t.Fatal(err)
		}
		e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
		r := &widget.Router{Root: e}
		r.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5})
		r.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
		return r, e
	}

	t.Run("printable keys reach the focused widget as text", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{text: map[uint32]string{30: "x"}}
		routeKey(tr, r, 30, 0, nil)
		if got := e.Text(); got != "x" {
			t.Errorf("text = %q, want x", got)
		}
	})

	t.Run("editing keysyms become actions", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{14: xkb.KeyBackSpace}}
		e.SetText("abc")
		routeKey(tr, r, 14, 0, nil)
		if got := e.Text(); got != "ab" {
			t.Errorf("text = %q, want ab", got)
		}
	})

	t.Run("ctrl+a selects all instead of typing", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{30: xkb.Keysym('a')}}
		e.SetText("abc")
		routeKey(tr, r, 30, 0, nil)
		routeKey(tr, r, 30, wlsessionCtrl(), nil)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 3 {
			t.Errorf("selection = %d..%d active=%v, want 0..3 true", start, end, active)
		}
	})

	t.Run("alt never produces text", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{text: map[uint32]string{30: "x"}}
		routeKey(tr, r, 30, wlsessionAlt(), nil)
		if got := e.Text(); got != "" {
			t.Errorf("alt+key typed %q", got)
		}
	})

	t.Run("the app hook sees every key", func(t *testing.T) {
		r, _ := newRouter()
		tr := &fakeTranslator{}
		seen := 0
		routeKey(tr, r, 1, 0, func(*widget.Router, uint32, wlsession.Mods) { seen++ })
		if seen != 1 {
			t.Errorf("hook calls = %d, want 1", seen)
		}
	})

	t.Run("shift+editing keys extend through to the action", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{106: xkb.KeyRight}}
		e.SetText("hello")
		e.MoveHome()
		routeKey(tr, r, 106, wlsessionShift(), nil)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 1 {
			t.Errorf("selection = %d..%d active=%v, want 0..1 true", start, end, active)
		}
	})
}

// wlsessionShift, wlsessionCtrl, and wlsessionAlt build modifier masks
// without importing the session package's unexported state.
func wlsessionShift() wlsession.Mods { return wlsession.ModShift }

func wlsessionCtrl() wlsession.Mods { return wlsession.ModCtrl }

func wlsessionAlt() wlsession.Mods { return wlsession.ModAlt }
