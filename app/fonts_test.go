package app

import "testing"

func TestFontVariants(t *testing.T) {
	base, err := Font("sans", 14)
	if err != nil {
		t.Skipf("no system sans: %v", err)
	}
	v := FontVariants(base)
	if v(false, false) != base {
		t.Error("the regular style is not the base face")
	}
	bold := v(true, false)
	if bold == nil {
		t.Fatal("no bold face")
	}
	want, err := FontVariant(base, true, false)
	if err == nil && want != nil && bold.Describe() != want.Describe() {
		t.Errorf("bold %+v, FontVariant %+v", bold.Describe(), want.Describe())
	}
	if want != nil && want.Describe() != base.Describe() && bold == base {
		t.Error("the family has a bold face but the variants serve the base")
	}
	if v(false, true) == nil || v(true, true) == nil {
		t.Error("an italic style resolved no face")
	}
}
