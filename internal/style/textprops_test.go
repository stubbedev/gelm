package style

// The CSS Text 3 and font-variant property tests: parse into the
// computed enum/flag/length, reject the invalid keywords, and pin the
// inheritance split (the text properties inherit, vertical-align,
// line-clamp, text-overflow, unicode-bidi and font-synthesis do not).

import "testing"

// parseWarns parses a stylesheet whose declarations are expected to
// warn: the invalid-value cases, which skip their declaration by
// design. The sink is swapped so the expected warnings never reach
// the test log.
func parseWarns(t *testing.T, css string) *Sheet {
	t.Helper()
	prev := parseWarn
	parseWarn = func(string) {}
	t.Cleanup(func() { parseWarn = prev })
	return Parse(css)
}

func TestTextAlign(t *testing.T) {
	s := parseOne(t, `
		label { text-align: center; }
		label.j { text-align: justify; }
		label.g { text-align: fill; }
	`)
	if got := computeTree(s, el("label")).TextAlign; got != AlignCenter {
		t.Errorf("text-align center = %v", got)
	}
	if got := computeTree(s, el("label", "j")).TextAlign; got != AlignJustify {
		t.Errorf("text-align justify = %v", got)
	}
	if got := computeTree(s, el("label", "g")).TextAlign; got != AlignJustify {
		t.Errorf("text-align fill (GTK) = %v, want justify", got)
	}
	bad := parseWarns(t, `label { text-align: middle; }`)
	if bv := computeTree(bad, el("label")); bv.Has(PropTextAlign) {
		t.Error("text-align: middle parsed")
	}
	root := el("label")
	root.add("span")
	if got := computeTree(s, root.kids[0]).TextAlign; got != AlignCenter {
		t.Errorf("inherited text-align = %v, want center", got)
	}
}

func TestTextIndentWordSpacing(t *testing.T) {
	s := parseOne(t, `
		label { font-size: 10px; text-indent: 2em; word-spacing: 4px; }
		label.w { word-spacing: 3px; }
	`)
	v := computeTree(s, el("label"))
	if v.TextIndent != 20 {
		t.Errorf("text-indent 2em = %v, want 20", v.TextIndent)
	}
	if v.WordSpacing != 4 {
		t.Errorf("word-spacing 4px = %v, want 4", v.WordSpacing)
	}
	if got := computeTree(s, el("label", "w")).WordSpacing; got != 3 {
		t.Errorf("word-spacing override = %v, want 3", got)
	}
	bad := parseWarns(t, `label { text-indent: 10%; }`)
	if cv := computeTree(bad, el("label")); cv.Has(PropTextIndent) {
		t.Error("text-indent: 10% parsed (percent needs the containing block)")
	}
}

func TestWhiteSpaceWordBreakLineBreakHyphens(t *testing.T) {
	s := parseOne(t, `
		label { white-space: pre-wrap; word-break: keep-all; line-break: strict; hyphens: manual; }
		label.b { white-space: break-spaces; word-break: break-all; line-break: anywhere; }
		label.n { hyphens: none; }
	`)
	v := computeTree(s, el("label"))
	if v.WhiteSpace != WhiteSpacePreWrap || v.WordBreak != WordBreakKeepAll ||
		v.LineBreak != LineBreakStrict || v.Hyphens != HyphensManual {
		t.Fatalf("keyword set = %v %v %v %v", v.WhiteSpace, v.WordBreak, v.LineBreak, v.Hyphens)
	}
	b := computeTree(s, el("label", "b"))
	if b.WhiteSpace != WhiteSpaceBreakSpaces || b.WordBreak != WordBreakBreakAll ||
		b.LineBreak != LineBreakAnywhere {
		t.Fatalf("override set = %v %v %v", b.WhiteSpace, b.WordBreak, b.LineBreak)
	}
	if got := computeTree(s, el("label", "n")).Hyphens; got != HyphensNone {
		t.Errorf("hyphens none = %v", got)
	}
	bad := parseWarns(t, `label { word-break: emergency; }`)
	if cv := computeTree(bad, el("label")); cv.Has(PropWordBreak) {
		t.Error("word-break: emergency parsed")
	}
	root := el("label")
	kid := root.add("span")
	kv := computeTree(s, kid)
	if !kv.Has(PropWhiteSpace) || kv.WhiteSpace != WhiteSpacePreWrap {
		t.Error("white-space does not inherit")
	}
}

func TestVerticalAlign(t *testing.T) {
	s := parseOne(t, `
		label { font-size: 10px; line-height: 3; vertical-align: sub; }
		label.sup { vertical-align: super; }
		label.px { vertical-align: -4px; }
		label.pc { vertical-align: 10%; }
	`)
	v := computeTree(s, el("label"))
	if v.VerticalAlign != VerticalAlignSub || v.VerticalAlignPx != 0 {
		t.Fatalf("vertical-align sub = %v %v", v.VerticalAlign, v.VerticalAlignPx)
	}
	if got := computeTree(s, el("label", "sup")).VerticalAlign; got != VerticalAlignSuper {
		t.Errorf("super = %v", got)
	}
	if got := computeTree(s, el("label", "px")).VerticalAlignPx; got != -4 {
		t.Errorf("-4px = %v, want -4", got)
	}
	// 10% of the 30px line-height, a fraction resolved against the
	// line box at paint.
	pcv := computeTree(s, el("label", "pc"))
	if pcv.VerticalAlignPx != 0.1 || !pcv.VerticalAlignPct {
		t.Errorf("10%% = %v pct=%v, want 0.1 pct=true", pcv.VerticalAlignPx, pcv.VerticalAlignPct)
	}
	root := el("label")
	kid := root.add("span")
	if cv := computeTree(s, kid); cv.Has(PropVerticalAlign) {
		t.Error("vertical-align inherited (it must not)")
	}
}

func TestLineClampTextOverflow(t *testing.T) {
	s := parseOne(t, `
		label { line-clamp: 3; text-overflow: ellipsis; }
		label.webkit { -webkit-line-clamp: 2; }
		label.clip { text-overflow: clip; }
		label.none { line-clamp: none; }
	`)
	v := computeTree(s, el("label"))
	if v.LineClamp != 3 || v.Overflow != TextOverflowEllipsis {
		t.Fatalf("clamp/overflow = %v %v", v.LineClamp, v.Overflow)
	}
	if got := computeTree(s, el("label", "webkit")).LineClamp; got != 2 {
		t.Errorf("-webkit-line-clamp = %v, want 2", got)
	}
	if got := computeTree(s, el("label", "clip")).Overflow; got != TextOverflowClip {
		t.Errorf("clip = %v", got)
	}
	if got := computeTree(s, el("label", "none")).LineClamp; got != 0 {
		t.Errorf("none = %v, want 0", got)
	}
	bad := parseWarns(t, `label { line-clamp: 1.5; }`)
	if cv := computeTree(bad, el("label")); cv.Has(PropLineClamp) {
		t.Error("line-clamp: 1.5 parsed")
	}
}

func TestFontVariant(t *testing.T) {
	s := parseOne(t, `
		label { font-variant: small-caps tabular-nums; }
		label.sub { font-variant-position: sub; }
		label.lig { font-variant-ligatures: discretionary-ligatures historical-ligatures; }
		label.ea { font-variant-east-asian: traditional full-width; }
		label.alt { font-variant-alternates: historical-forms; }
		label.caps { font-variant-caps: all-small-caps; }
		label.num { font-variant-numeric: oldstyle-nums slashed-zero ordinal; }
	`)
	v := computeTree(s, el("label"))
	if v.VariantCaps != CapsSmallCaps || v.VariantNumeric != NumTabular {
		t.Fatalf("shorthand = %v %v", v.VariantCaps, v.VariantNumeric)
	}
	if got := computeTree(s, el("label", "sub")).VariantPos; got != PositionSub {
		t.Errorf("position = %v", got)
	}
	lig := computeTree(s, el("label", "lig")).VariantLig
	if lig&LigDiscretionary == 0 || lig&LigHistorical == 0 || lig&LigCommon != 0 {
		t.Errorf("ligatures = %b", lig)
	}
	ea := computeTree(s, el("label", "ea")).VariantEA
	if ea&EATraditional == 0 || ea&EAFullWidth == 0 {
		t.Errorf("east-asian = %b", ea)
	}
	if got := computeTree(s, el("label", "alt")).VariantAlt; got != AlternatesHistorical {
		t.Errorf("alternates = %v", got)
	}
	if got := computeTree(s, el("label", "caps")).VariantCaps; got != CapsAllSmallCaps {
		t.Errorf("caps = %v", got)
	}
	num := computeTree(s, el("label", "num")).VariantNumeric
	if num&NumOldStyle == 0 || num&NumSlashedZero == 0 || num&NumOrdinal == 0 {
		t.Errorf("numeric = %b", num)
	}
	bad := parseWarns(t, `label { font-variant: swash(fancy); }`)
	if bv := computeTree(bad, el("label")); bv.Has(PropFontVariantCaps) {
		t.Error("font-variant: swash(fancy) parsed")
	}
	root := el("label")
	kid := root.add("span")
	if kv := computeTree(s, kid); !kv.Has(PropFontVariantCaps) {
		t.Error("font-variant does not inherit")
	}
}

func TestFontSynthesisOpticalSizing(t *testing.T) {
	s := parseOne(t, `
		label { font-synthesis: none; font-optical-sizing: none; }
		label.some { font-synthesis: weight small-caps style; }
	`)
	v := computeTree(s, el("label"))
	if v.Synthesis != 0 || v.OpticalSizing {
		t.Fatalf("none/none = %v %v", v.Synthesis, v.OpticalSizing)
	}
	w := computeTree(s, el("label", "some"))
	if w.Synthesis != SynthWeight|SynthSmallCaps|SynthStyle || w.OpticalSizing {
		t.Fatalf("weight+small-caps+style auto = %v", w.Synthesis)
	}
	root := el("label")
	kid := root.add("span")
	if cv := computeTree(s, kid); cv.Has(PropFontSynthesis) {
		t.Error("font-synthesis inherited (it must not)")
	}
}

func TestDirectionUnicodeBidiWritingMode(t *testing.T) {
	s := parseOne(t, `
		label { direction: rtl; unicode-bidi: isolate; writing-mode: vertical-rl; }
		label.ltr { direction: ltr; unicode-bidi: bidi-override; }
	`)
	v := computeTree(s, el("label"))
	if v.Direction != 2 || v.UnicodeBidi != BidiIsolate || v.WritingMode != WritingVerticalRL {
		t.Fatalf("rtl/isolate/vertical-rl = %v %v %v", v.Direction, v.UnicodeBidi, v.WritingMode)
	}
	l := computeTree(s, el("label", "ltr"))
	if l.Direction != 1 || l.UnicodeBidi != BidiOverride {
		t.Fatalf("ltr/override = %v %v", l.Direction, l.UnicodeBidi)
	}
	bad := parseWarns(t, `label { direction: auto; }`)
	if cv := computeTree(bad, el("label")); cv.Has(PropDirection) {
		t.Error("direction: auto parsed")
	}
	root := el("label")
	kid := root.add("span")
	kv := computeTree(s, kid)
	if !kv.Has(PropDirection) || kv.Direction != 2 {
		t.Error("direction does not inherit")
	}
	if kv.Has(PropUnicodeBidi) {
		t.Error("unicode-bidi inherited (it must not)")
	}
}
