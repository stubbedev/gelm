package style

import (
	"os"
	"testing"
	"time"
)

func TestProbeWayle(t *testing.T) {
	b, err := os.ReadFile("/tmp/claude-1000/-home-stubbe-git-private-wayle/73d5b916-9649-4e57-af10-c2c00cfbb366/scratchpad/style.css")
	if err != nil {
		t.Skip()
	}
	var warns []string
	prev := parseWarn
	parseWarn = func(m string) { warns = append(warns, m) }
	defer func() { parseWarn = prev }()
	start := time.Now()
	s := Parse(string(b))
	t.Logf("parse %v, %d rules, %d warns", time.Since(start), len(s.rules), len(warns))
	for _, w := range warns {
		t.Log(w)
	}
	bar := ParseDeclarations(`--bar-scale: 1; --bar-bg: var(--bg-surface); --bar-opacity: 100%; --bar-border-color: var(--border-accent); --bar-border-top: 0; --bar-border-bottom: 2; --bar-border-left: 0; --bar-border-right: 0; --bar-inset-edge-px: 0; --bar-inset-ends-px: 0; --bar-padding-px: 6; --bar-padding-ends-px: 8; --bar-module-gap-px: 8; --bar-button-opacity: 1; --bar-button-bg-opacity: 100%; --bar-btn-label-weight: var(--weight-normal); --bar-group-module-gap-px: 4; --bar-group-padding-px: 0; --bar-group-bg: var(--bg-elevated); --bar-group-opacity: 100%; --bar-group-border-color: var(--border-accent); --bar-group-border-top: 0; --bar-group-border-bottom: 0; --bar-group-border-left: 0; --bar-group-border-right: 0; --bar-shadow: none; --bar-shadow-margin: 0;`)
	btnVars := ParseDeclarations(`--bar-btn-icon-color: var(--fg-default); --bar-btn-label-color: var(--yellow); --bar-btn-icon-bg: var(--yellow); --bar-btn-bg: var(--bg-surface-elevated); --bar-btn-border-color: var(--yellow); --bar-btn-border-width: 1px;`)
	win := &tnode{t: Target{Element: "window", Classes: []string{"bar", "top"}, Inline: bar, InlinePriority: PriorityUser}}
	section := win.child("box", "bar-section", "bar-left")
	item := section.child("box", "bar-item")
	mb := item.child("menubutton", "bar-button", "basic", "module")
	mb.t.Inline = btnVars
	mb.t.InlinePriority = PriorityUser
	tog := mb.child("button", "toggle")
	content := tog.child("box", "bar-button-content")
	ic := content.child("box", "icon-container")
	img := ic.child("image")
	lc := content.child("box", "label-container")
	lbl := lc.child("label", "bar-button-label")
	layers := []Layer{{Sheet: s, Priority: PriorityUser + 100}}
	var sc Scratch
	start = time.Now()
	var vals []*Values
	var prevV *Values
	for _, n := range []*tnode{win, section, item, mb, tog, content, ic, img, lc, lbl} {
		v := &Values{}
		Compute(layers, n, prevV, nil, Env{Rem: 16}, &sc, v)
		vals = append(vals, v)
		prevV = v
	}
	t.Logf("compute chain %v", time.Since(start))
	for i, n := range []string{"win", "section", "item", "mb", "tog", "content", "ic", "img", "lc", "lbl"} {
		v := vals[i]
		t.Logf("%s: set=%x color=%08x bg=%08x pad=%v margin=%v bw=%v radius=%v font=%v/%v/%q icon=%d op=%v shadow=%v", n, uint64(v.Set), uint32(v.Color), uint32(v.Background), v.Padding, v.Margin, v.EffBorder(), v.Radius, v.FontSize, v.FontWeight, v.FontFamily, v.IconSize, v.Opacity, v.Shadow.List())
	}
}

type tnode struct {
	t      Target
	parent *tnode
	kids   []*tnode
}

func (n *tnode) child(el string, classes ...string) *tnode {
	c := &tnode{t: Target{Element: el, Classes: classes}, parent: n}
	n.kids = append(n.kids, c)
	return c
}
func (n *tnode) StyleTarget(t *Target) { *t = n.t }
func (n *tnode) StyleParent() Node {
	if n.parent == nil {
		return nil
	}
	return n.parent
}
func (n *tnode) StylePrev() Node {
	if n.parent == nil {
		return nil
	}
	for i, k := range n.parent.kids {
		if k == n && i > 0 {
			return n.parent.kids[i-1]
		}
	}
	return nil
}
func (n *tnode) StylePosition() (int, int) {
	if n.parent == nil {
		return 1, 1
	}
	for i, k := range n.parent.kids {
		if k == n {
			return i + 1, len(n.parent.kids)
		}
	}
	return 1, 1
}
