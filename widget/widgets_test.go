package widget

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/render"
)

func entryFace(t *testing.T) *render.Typeface {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func TestEntryEditing(t *testing.T) {
	newEntry := func() *Entry {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("abc")
		return e
	}

	t.Run("insert puts text at the cursor and advances it", func(t *testing.T) {
		e := newEntry()
		e.MoveHome()
		e.Insert("X")
		if e.Text() != "Xabc" {
			t.Errorf("text = %q, want Xabc", e.Text())
		}
		if e.Cursor() != 1 {
			t.Errorf("cursor = %d, want 1", e.Cursor())
		}
		e.MoveCursor(3)
		e.Insert("Z")
		if e.Text() != "XabcZ" {
			t.Errorf("text = %q, want XabcZ", e.Text())
		}
	})

	t.Run("backspace removes the rune before the cursor", func(t *testing.T) {
		e := newEntry()
		e.MoveCursor(-1)
		e.Backspace()
		if e.Text() != "ac" {
			t.Errorf("text = %q, want ac", e.Text())
		}
		if e.Cursor() != 1 {
			t.Errorf("cursor = %d, want 1", e.Cursor())
		}
	})

	t.Run("backspace at the start is a no-op", func(t *testing.T) {
		e := newEntry()
		e.MoveHome()
		e.Backspace()
		if e.Text() != "abc" || e.Cursor() != 0 {
			t.Errorf("text = %q cursor = %d, want abc/0", e.Text(), e.Cursor())
		}
	})

	t.Run("delete removes the rune at the cursor", func(t *testing.T) {
		e := newEntry()
		e.MoveHome()
		e.Delete()
		if e.Text() != "bc" || e.Cursor() != 0 {
			t.Errorf("text = %q cursor = %d, want bc/0", e.Text(), e.Cursor())
		}
	})

	t.Run("delete at the end is a no-op", func(t *testing.T) {
		e := newEntry()
		e.MoveEnd()
		e.Delete()
		if e.Text() != "abc" || e.Cursor() != 3 {
			t.Errorf("text = %q cursor = %d, want abc/3", e.Text(), e.Cursor())
		}
	})

	t.Run("cursor motion clamps at both ends", func(t *testing.T) {
		e := newEntry()
		e.MoveCursor(-5)
		if e.Cursor() != 0 {
			t.Errorf("cursor = %d, want 0", e.Cursor())
		}
		e.MoveCursor(99)
		if e.Cursor() != 3 {
			t.Errorf("cursor = %d, want 3", e.Cursor())
		}
	})

	t.Run("home and end", func(t *testing.T) {
		e := newEntry()
		e.MoveEnd()
		if e.Cursor() != 3 {
			t.Errorf("end cursor = %d", e.Cursor())
		}
		e.MoveHome()
		if e.Cursor() != 0 {
			t.Errorf("home cursor = %d", e.Cursor())
		}
	})

	t.Run("insert works on an empty entry and with multibyte runes", func(t *testing.T) {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.Insert("héllo")
		if e.Text() != "héllo" {
			t.Errorf("text = %q", e.Text())
		}
		if e.Cursor() != 5 {
			t.Errorf("cursor = %d, want 5 (rune count)", e.Cursor())
		}
	})
}

func TestEntryMeasurePaint(t *testing.T) {
	face := entryFace(t)

	t.Run("placeholder gives an empty entry width", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetPlaceholder("type here")
		got := e.Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W <= 0 {
			t.Errorf("placeholder entry width = %d, want > 0", got.W)
		}
	})

	t.Run("empty entry without placeholder wants only padding", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		got := e.Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W != 16 {
			t.Errorf("empty entry width = %d, want 16", got.W)
		}
	})

	t.Run("painting an empty entry shows no text ink beyond the cursor", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.Measure(Constraints{Max: Size{W: 500, H: 100}})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 28})
		cv := render.New(make([]byte, render.Stride(200)*28), render.Stride(200), 200, 28)
		e.Paint(cv)
	})
}

func TestSliderValueModel(t *testing.T) {
	t.Run("constructor clamps the initial value", func(t *testing.T) {
		if got := NewSlider(0, 10, 0, 99).Value(); got != 10 {
			t.Errorf("value = %v, want 10", got)
		}
		if got := NewSlider(0, 10, 0, -3).Value(); got != 0 {
			t.Errorf("value = %v, want 0", got)
		}
	})

	t.Run("SetValue clamps", func(t *testing.T) {
		s := NewSlider(0, 10, 0, 5)
		s.SetValue(42)
		if s.Value() != 10 {
			t.Errorf("value = %v, want 10", s.Value())
		}
	})

	t.Run("step snaps", func(t *testing.T) {
		s := NewSlider(0, 10, 2.5, 0)
		s.SetValue(3.2)
		if s.Value() != 2.5 {
			t.Errorf("value = %v, want 2.5", s.Value())
		}
	})

	t.Run("OnChanged fires only when the value moved", func(t *testing.T) {
		fired := 0
		s := NewSlider(0, 10, 0, 5)
		s.OnChanged = func(float64) { fired++ }
		s.SetValue(6)
		s.SetValue(6)
		if fired != 1 {
			t.Errorf("fired %d times, want 1", fired)
		}
	})

	t.Run("ValueFromX maps the trough to values with edge padding", func(t *testing.T) {
		s := NewSlider(0, 100, 0, 0)
		s.Measure(Constraints{Max: Size{W: 500, H: 100}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 204, H: 18})
		if got := s.ValueFromX(4); got != 0 {
			t.Errorf("left edge = %v, want 0", got)
		}
		if got := s.ValueFromX(200); got != 100 {
			t.Errorf("right edge = %v, want 100", got)
		}
		if got := s.ValueFromX(102); got < 49 || got > 51 {
			t.Errorf("middle = %v, want ~50", got)
		}
		if got := s.ValueFromX(-100); got != 0 {
			t.Errorf("far left = %v, want clamped 0", got)
		}
	})

	t.Run("degenerate min==max never divides by zero", func(t *testing.T) {
		s := NewSlider(5, 5, 0, 5)
		if got := s.ValueFromX(100); got != 5 {
			t.Errorf("value = %v, want 5", got)
		}
	})
}

func TestSwitchAndCheck(t *testing.T) {
	t.Run("SetOn fires only on flips", func(t *testing.T) {
		fired := 0
		s := NewSwitch(false)
		s.OnChanged = func(bool) { fired++ }
		s.SetOn(true)
		s.SetOn(true)
		if fired != 1 || !s.On() {
			t.Errorf("fired = %d, on = %v", fired, s.On())
		}
	})

	t.Run("Toggle flips", func(t *testing.T) {
		s := NewSwitch(false)
		s.Toggle()
		if !s.On() {
			t.Error("toggle did not turn on")
		}
		s.Toggle()
		if s.On() {
			t.Error("toggle did not turn off")
		}
	})

	t.Run("check button toggles and fires on flips only", func(t *testing.T) {
		fired := 0
		c := NewCheckButton(false)
		c.OnChanged = func(bool) { fired++ }
		c.Toggle()
		c.SetChecked(true)
		if !c.Checked() || fired != 1 {
			t.Errorf("checked = %v, fired = %d", c.Checked(), fired)
		}
	})

	t.Run("paint distinguishes on and off", func(t *testing.T) {
		paint := func(on bool) render.Color {
			data := make([]byte, render.Stride(40)*22)
			cv := render.New(data, render.Stride(40), 40, 22)
			s := NewSwitch(on)
			s.Measure(Constraints{Max: Size{W: 100, H: 100}})
			s.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 22})
			s.Paint(cv)
			return render.ColorFromBytes(data[11*render.Stride(40)+3*4:])
		}
		off := paint(false)
		on := paint(true)
		if on == off {
			t.Error("on and off tracks paint identically")
		}
	})
}

func TestProgressBar(t *testing.T) {
	t.Run("value clamps to [0, 1]", func(t *testing.T) {
		p := NewProgressBar(5)
		if p.Value() != 1 {
			t.Errorf("value = %v, want 1", p.Value())
		}
		p.SetValue(-1)
		if p.Value() != 0 {
			t.Errorf("value = %v, want 0", p.Value())
		}
		p.SetValue(0.5)
		if p.Value() != 0.5 {
			t.Errorf("value = %v, want 0.5", p.Value())
		}
	})
}

func TestStack(t *testing.T) {
	newStack := func() *Stack {
		return NewStack().
			Add("a", newStub(10, 10)).
			Add("b", newStub(30, 5))
	}

	t.Run("first added child is visible", func(t *testing.T) {
		if got := newStack().Visible(); got != "a" {
			t.Errorf("visible = %q, want a", got)
		}
	})

	t.Run("measure takes the largest child", func(t *testing.T) {
		got := newStack().Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{W: 30, H: 10}) {
			t.Errorf("stack measure = %v, want 30x10", got)
		}
	})

	t.Run("show switches the painted child", func(t *testing.T) {
		s := newStack()
		s.Show("b")
		if s.Visible() != "b" {
			t.Fatalf("visible = %q, want b", s.Visible())
		}
	})

	t.Run("show ignores unknown names", func(t *testing.T) {
		s := newStack()
		s.Show("nope")
		if s.Visible() != "a" {
			t.Errorf("visible = %q, want still a", s.Visible())
		}
	})

	t.Run("hit test respects visibility", func(t *testing.T) {
		s := NewStack().Add("a", newStub(10, 10)).Add("b", newStub(10, 10))
		s.Measure(Constraints{Max: Size{W: 100, H: 100}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})
		if got := s.HitTest(Point{X: 5, Y: 5}); got != s.kids["a"] {
			t.Errorf("hit = %v, want child a", got)
		}
	})
}

func TestOverlay(t *testing.T) {
	o := NewOverlay().Append(newStub(10, 10)).Append(newStub(5, 5))
	o.Measure(Constraints{Max: Size{W: 100, H: 100}})
	o.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

	t.Run("measure takes the largest child", func(t *testing.T) {
		if got := o.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 10, H: 10}) {
			t.Errorf("overlay measure = %v", got)
		}
	})

	t.Run("topmost child wins the hit test", func(t *testing.T) {
		bottom, top := newStub(20, 20), newStub(20, 20)
		ov := NewOverlay().Append(bottom).Append(top)
		ov.Measure(Constraints{Max: Size{W: 100, H: 100}})
		ov.Arrange(render.Rect{X: 0, Y: 0, W: 30, H: 30})
		if got := ov.HitTest(Point{X: 5, Y: 5}); got != top {
			t.Errorf("hit = %v, want top", got)
		}
	})
}

func TestScroll(t *testing.T) {
	tall := NewBox(Column, 0, 0)
	for range 10 {
		tall.Append(newStub(50, 20), false)
	}

	newScroll := func() *Scroll {
		s := NewScroll(tall)
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		return s
	}

	t.Run("viewport fits the constraints, child keeps its natural size", func(t *testing.T) {
		s := newScroll()
		if got := s.Measure(Constraints{Max: Size{W: 60, H: 60}}); got != (Size{W: 60, H: 60}) {
			t.Errorf("viewport measure = %v, want 60x60", got)
		}
		if s.nat.H != 200 {
			t.Errorf("child natural height = %d, want 200", s.nat.H)
		}
	})

	t.Run("offset clamps to the overflow", func(t *testing.T) {
		s := newScroll()
		s.SetOffset(0, 500)
		if s.offY != 140 {
			t.Errorf("offset = %d, want clamped 140", s.offY)
		}
		s.SetOffset(0, -10)
		if s.offY != 0 {
			t.Errorf("offset = %d, want 0", s.offY)
		}
	})

	t.Run("child smaller than the viewport stays pinned", func(t *testing.T) {
		s := NewScroll(newStub(10, 10))
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		s.SetOffset(5, 5)
		if s.offX != 0 || s.offY != 0 {
			t.Errorf("offset = %d,%d, want 0,0", s.offX, s.offY)
		}
	})

	t.Run("hits inside land on the child, empty area on the scroll", func(t *testing.T) {
		s := NewScroll(newStub(20, 20))
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		if got := s.HitTest(Point{X: 10, Y: 10}); got != s.child {
			t.Errorf("hit = %v, want the child", got)
		}
		if got := s.HitTest(Point{X: 40, Y: 40}); got != Widget(s) {
			t.Errorf("hit over empty viewport = %v, want the scroll", got)
		}
		if got := s.HitTest(Point{X: 500, Y: 500}); got != nil {
			t.Errorf("hit outside = %v, want nil", got)
		}
	})
}

func TestSpacer(t *testing.T) {
	s := NewSpacer(12, 4)
	if got := s.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 12, H: 4}) {
		t.Errorf("spacer = %v, want 12x4", got)
	}
	if got := s.HitTest(Point{X: 1, Y: 1}); got != nil {
		t.Errorf("spacer hit = %v, want nil", got)
	}
}

func TestCanvasLine(t *testing.T) {
	stride := render.Stride(20)
	data := make([]byte, stride*20)
	cv := render.New(data, stride, 20, 20)
	cv.Line(2, 10, 18, 10, 3, render.RGB(255, 0, 0))
	hits := 0
	for x := range 20 {
		if render.ColorFromBytes(data[10*stride+x*4:]).A() > 0 {
			hits++
		}
	}
	if hits < 15 {
		t.Errorf("horizontal line only %d px wide, want >= 15", hits)
	}
}
