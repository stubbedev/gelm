package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

func paintSwitchTrack(t *testing.T) render.Color {
	t.Helper()
	data := make([]byte, render.Stride(40)*22)
	cv := render.New(data, render.Stride(40), 40, 22)
	s := NewSwitch(false)
	s.Measure(Constraints{Max: Size{W: 100, H: 100}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 22})
	s.Paint(cv)
	// (38, 11) is inside the track, clear of the knob.
	return render.ColorFromBytes(data[11*render.Stride(40)+38*4:])
}

func TestThemeRestyles(t *testing.T) {
	defer SetTheme(DarkTheme())

	t.Run("swapping the theme repaints controls", func(t *testing.T) {
		SetTheme(DarkTheme())
		dark := paintSwitchTrack(t)
		SetTheme(LightTheme())
		light := paintSwitchTrack(t)
		SetTheme(DarkTheme())
		again := paintSwitchTrack(t)

		if dark == light {
			t.Errorf("switch track identical under dark and light themes: %v", dark)
		}
		if again != dark {
			t.Errorf("restoring the theme did not restore colors: %v != %v", again, dark)
		}
	})

	t.Run("track follows theme surface, not a hardcoded color", func(t *testing.T) {
		SetTheme(DarkTheme())
		if got := paintSwitchTrack(t); got != DarkTheme().Surface {
			t.Errorf("track = %v, want theme surface", got)
		}
	})
}

func TestThemeExplicitColorWins(t *testing.T) {
	defer SetTheme(DarkTheme())

	green := render.RGB(0, 200, 0)
	b := NewButton(newStub(0, 0), 0, 0)
	b.Bg = green
	b.Measure(Constraints{Max: Size{W: 100, H: 100}})
	b.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 20})

	paint := func() render.Color {
		data := make([]byte, render.Stride(20)*20)
		cv := render.New(data, render.Stride(20), 20, 20)
		b.Hovered, b.Pressed = false, false
		b.Paint(cv)
		return render.ColorFromBytes(data[10*render.Stride(20)+10*4:])
	}

	SetTheme(DarkTheme())
	dark := paint()
	SetTheme(LightTheme())
	light := paint()

	t.Run("explicit colors survive a theme swap", func(t *testing.T) {
		if dark != green || light != green {
			t.Errorf("explicit Bg = %v (dark), %v (light), want %v", dark, light, green)
		}
	})
}

func TestThemeDefaults(t *testing.T) {
	t.Run("SetTheme nil restores dark", func(t *testing.T) {
		SetTheme(LightTheme())
		SetTheme(nil)
		if Current().Bg != DarkTheme().Bg {
			t.Errorf("bg = %v, want the dark theme bg", Current().Bg)
		}
	})
}
