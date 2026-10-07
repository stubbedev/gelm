package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// The trough and fill style as `progressbar > trough > progress`: the
// trough's min sizes size the bar and its background paints under the
// fill; the progress node paints the filled fraction.
func TestProgressBarNodes(t *testing.T) {
	loadCSS(t, `progressbar trough { min-height: 8; background-color: #010203; } progressbar trough progress { background-color: #0a0b0c; }`)
	p := NewProgressBar(0.5)
	host := NewBox(Column, 0, 0)
	host.Append(p, false)
	frame(t, host, 200, 40)
	if got := p.Measure(Constraints{Max: Size{W: 200, H: 40}}); got.H != 8 {
		t.Errorf("bar height %d, want the trough's min-height 8", got.H)
	}
	tv := p.trough.style(&p.trough)
	if got := tv.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("trough background %v, want the trough rule", got)
	}
	pv := p.trough.fill.style(&p.trough.fill)
	if got := pv.Background; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("progress background %v, want the progress rule", got)
	}
	if fr := p.trough.fill.Bounds(); fr.W*2 != p.trough.Bounds().W || fr.H != 8 {
		t.Errorf("fill rect %+v, want half the trough wide at min-height 8", fr)
	}
}

// Unstyled, the bar is the theme's: a 160x10 surface trough with the
// accent fill, unchanged.
func TestProgressBarUnstyledDefaults(t *testing.T) {
	p := NewProgressBar(0.5)
	arrangeTree(t, p, 200, 40)
	if got := p.Measure(Constraints{Max: Size{W: 200, H: 40}}); got.W != 160 || got.H != 10 {
		t.Errorf("bar %v, want the 160x10 default", got)
	}
	th := Current()
	if got := p.troughFill(th); got != th.Surface {
		t.Errorf("trough fallback %v, want the theme surface", got)
	}
	if got := p.fillFill(th); got != th.Accent {
		t.Errorf("progress fallback %v, want the theme accent", got)
	}
	p.Fill, p.Trough = render.RGB(1, 2, 3), render.RGB(4, 5, 6)
	if got := p.fillFill(th); got != render.RGB(1, 2, 3) {
		t.Errorf("programmatic fill %v, want the set color", got)
	}
	if got := p.troughFill(th); got != render.RGB(4, 5, 6) {
		t.Errorf("programmatic trough %v, want the set color", got)
	}
}
