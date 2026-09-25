package app

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/internal/wlsession"
)

func TestKeyRepeater(t *testing.T) {
	t.Run("no repeat before the initial delay", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, wlsession.ModShift)
		if _, _, ok := r.tick(); ok {
			t.Error("tick fired before the delay elapsed")
		}
	})

	t.Run("one repeat after the delay, then at the rate", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.next = time.Now().Add(-time.Millisecond)
		code, _, ok := r.tick()
		if !ok || code != 30 {
			t.Errorf("tick = %d, %v, want 30, true", code, ok)
		}
		if _, _, ok := r.tick(); ok {
			t.Error("second tick fired immediately after the first repeat")
		}
		r.next = time.Now().Add(-time.Millisecond)
		if _, _, ok = r.tick(); !ok {
			t.Error("tick did not fire at the repeat rate")
		}
	})

	t.Run("release stops repeats", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.release(30)
		r.next = time.Now().Add(-time.Millisecond)
		if _, _, ok := r.tick(); ok {
			t.Error("tick fired after release")
		}
	})

	t.Run("a different key release does not stop the held key", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.release(31)
		r.next = time.Now().Add(-time.Millisecond)
		if _, _, ok := r.tick(); !ok {
			t.Error("unrelated release cancelled the held key's repeat")
		}
	})

	t.Run("a new press re-arms the delay", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.next = time.Now().Add(-time.Millisecond)
		r.press(30, 0)
		if _, _, ok := r.tick(); ok {
			t.Error("re-press fired immediately instead of re-arming the delay")
		}
	})

	t.Run("zeros fall back to usable defaults", func(t *testing.T) {
		r := newKeyRepeater(0, 0)
		if r.rate <= 0 || r.delay <= 0 {
			t.Errorf("defaults invalid: rate %v delay %v", r.rate, r.delay)
		}
	})

	t.Run("repeats carry the mods held at press time", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, wlsession.ModCtrl)
		r.next = time.Now().Add(-time.Millisecond)
		_, mods, ok := r.tick()
		if !ok || mods != wlsession.ModCtrl {
			t.Errorf("repeat mods = %v, %v, want ctrl", mods, ok)
		}
	})
}
