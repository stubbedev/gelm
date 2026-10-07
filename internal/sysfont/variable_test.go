package sysfont

import (
	"os"
	"testing"

	"github.com/go-text/typesetting/font"

	"github.com/stubbedev/gelm/render"
)

// A variable face the store matched at its default instance serves a
// requested weight through its wght axis; an already-matching weight
// and a static face come back unchanged.
func TestInstantiateVariable(t *testing.T) {
	data, err := os.ReadFile("../../render/testdata/Quicksand-VF.ttf")
	if err != nil {
		t.Fatal(err)
	}
	vf, err := render.LoadFont(data)
	if err != nil {
		t.Fatal(err)
	}
	def := vf.Describe().Aspect.Weight
	bold := instantiate(vf, font.Aspect{Style: font.StyleNormal, Weight: font.WeightBold})
	if bold == vf || len(bold.Variations()) != 1 || bold.Variations()[0].Value != 700 {
		t.Errorf("bold = %v", bold.Variations())
	}
	if same := instantiate(vf, font.Aspect{Style: font.StyleNormal, Weight: def}); same != vf {
		t.Error("the described weight made an instance")
	}
	static, _ := render.NewFixtureTypeface()
	if instantiate(static, font.Aspect{Weight: font.WeightBold}) != static {
		t.Error("a static face was instanced")
	}
}
