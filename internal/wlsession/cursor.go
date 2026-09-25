package wlsession

import (
	"fmt"
	"sync"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlcursor"
)

// defaultCursor is shown whenever the app has not asked for another
// shape: the classic arrow.
const defaultCursor = wlcursor.LeftPtr

// cursorThemeSize is the nominal cursor size in pixels; XCURSOR_SIZE
// overrides it through wlcursor.
const cursorThemeSize = 24

var (
	cursorOnce sync.Once
	cursorThm  *wlcursor.Theme
	cursorErr  error
)

// loadCursorTheme parses the system xcursor theme once per process.
func loadCursorTheme(shm *wl.Shm) (*wlcursor.Theme, error) {
	cursorOnce.Do(func() {
		cursorThm, cursorErr = wlcursor.LoadTheme(cursorThemeSize, shm)
	})
	return cursorThm, cursorErr
}

// SetCursor shows the named xcursor shape (wlcursor.LeftPtr, Xterm,
// Grabbing, ...) on the pointer. The choice sticks across pointer
// enters; "" or an unknown name falls back to the arrow. It is a no-op
// before the pointer or the compositor surfaces exist.
func (s *Session) SetCursor(name string) error {
	s.desiredCursor = effectiveCursor(name)
	return s.applyCursor()
}

// effectiveCursor resolves a requested name to a real shape.
func effectiveCursor(desired string) string {
	if desired == "" {
		return defaultCursor
	}
	return desired
}

// applyCursor pushes the current cursor shape to the compositor: one
// shm buffer from the theme, attached to a dedicated cursor surface.
func (s *Session) applyCursor() error {
	if s.pointer == nil || s.pointerEnterSerial == 0 {
		return nil
	}
	if s.cursorSurface == nil {
		surf, err := s.Compositor().CreateSurface()
		if err != nil {
			return fmt.Errorf("cursor surface: %w", err)
		}
		s.cursorSurface = surf
	}
	theme, err := loadCursorTheme(s.shm)
	if err != nil {
		return fmt.Errorf("cursor theme: %w", err)
	}
	cur, err := theme.GetCursor(s.desiredCursor)
	if err != nil {
		// Unknown shape: fall back rather than failing the call.
		cur, err = theme.GetCursor(defaultCursor)
		if err != nil {
			return fmt.Errorf("default cursor: %w", err)
		}
	}
	if cur.ImageCount() == 0 {
		return nil
	}
	img := cur.GetCursorImage(0)
	if err := s.cursorSurface.Attach(img.GetBuffer(), 0, 0); err != nil {
		return fmt.Errorf("cursor attach: %w", err)
	}
	if err := s.cursorSurface.DamageBuffer(0, 0, int32(img.GetWidth()), int32(img.GetHeight())); err != nil {
		return fmt.Errorf("cursor damage: %w", err)
	}
	if err := s.cursorSurface.Commit(); err != nil {
		return fmt.Errorf("cursor commit: %w", err)
	}
	if err := s.pointer.SetCursor(s.pointerEnterSerial, s.cursorSurface,
		int32(img.GetHotspotX()), int32(img.GetHotspotY())); err != nil {
		return fmt.Errorf("set cursor: %w", err)
	}
	return nil
}
