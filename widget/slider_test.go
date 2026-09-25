package widget

import "testing"

func TestSliderKeyboard(t *testing.T) {
	t.Run("arrows nudge by the step", func(t *testing.T) {
		s := NewSlider(0, 100, 5, 50)
		s.KeyAction(KeyRight, 0)
		if got := s.Value(); got != 55 {
			t.Errorf("after right = %v, want 55", got)
		}
		s.KeyAction(KeyLeft, 0)
		s.KeyAction(KeyLeft, 0)
		if got := s.Value(); got != 45 {
			t.Errorf("after lefts = %v, want 45", got)
		}
	})

	t.Run("home and end jump to the extremes", func(t *testing.T) {
		s := NewSlider(10, 20, 0, 15)
		s.KeyAction(KeyHome, 0)
		if got := s.Value(); got != 10 {
			t.Errorf("home = %v, want 10", got)
		}
		s.KeyAction(KeyEnd, 0)
		if got := s.Value(); got != 20 {
			t.Errorf("end = %v, want 20", got)
		}
	})

	t.Run("without a step a tenth of the range nudges", func(t *testing.T) {
		s := NewSlider(0, 100, 0, 50)
		s.KeyAction(KeyRight, 0)
		if got := s.Value(); got != 60 {
			t.Errorf("after right = %v, want 60", got)
		}
	})
}
