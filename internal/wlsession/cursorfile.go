package wlsession

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/third_party/neurlang-wayland/wlcursor"
	"github.com/stubbedev/gelm/third_party/neurlang-wayland/wlcursor/xcursor"
)

// File cursors (CSS cursor: url()): the stylesheet names an xcursor
// file and a hotspot; the session loads its images straight off disk,
// no theme involved. Only one frame shows — the largest image the
// file carries — so an animated file cursor pins its opening frame;
// the theme path still advances the shapes it knows.

// fileCursorPrefix marks a SetCursor name that asks for a file
// cursor: `file:<path>#<x>,<y>`, the hotspot the stylesheet named
// replacing the file's own.
const fileCursorPrefix = "file:"

// parseFileCursor splits a file cursor request into its path and
// hotspot; ok reports the name is one.
func parseFileCursor(name string) (path string, hx, hy int, ok bool) {
	rest, found := strings.CutPrefix(name, fileCursorPrefix)
	if !found {
		return "", 0, 0, false
	}
	path, hot, found := strings.Cut(rest, "#")
	if !found || path == "" {
		return "", 0, 0, false
	}
	xs, ys, found := strings.Cut(hot, ",")
	if !found {
		return "", 0, 0, false
	}
	var err error
	if hx, err = strconv.Atoi(xs); err != nil || hx < 0 {
		return "", 0, 0, false
	}
	if hy, err = strconv.Atoi(ys); err != nil || hy < 0 {
		return "", 0, 0, false
	}
	return path, hx, hy, true
}

// pushFileCursorLocked shows the widest image of the xcursor file at
// path with the CSS hotspot. The theme's shm pool backs the buffer,
// so a theme must load; a missing or malformed file degrades to the
// arrow, the theme path's own rule. Called with cursorState.mu held.
func (s *Session) pushFileCursorLocked(path string, hx, hy int) error {
	img, err := fileCursorImage(path)
	if err != nil {
		debug.Log("input", "file cursor %q: %v", path, err)
		return s.fallbackFileCursorLocked()
	}
	theme, err := loadCursorTheme(s.shm)
	if err != nil {
		debug.Log("input", "file cursor theme: %v", err)
		return s.fallbackFileCursorLocked()
	}
	ib, err := wlcursor.NewImageBuffer(theme, img)
	if err != nil {
		debug.Log("input", "file cursor buffer: %v", err)
		return s.fallbackFileCursorLocked()
	}
	if s.crs.push != nil {
		return s.crs.push(ib)
	}
	if err := s.crs.surface.Attach(ib.GetBuffer(), 0, 0); err != nil {
		return fmt.Errorf("cursor attach: %w", err)
	}
	if err := s.crs.surface.DamageBuffer(0, 0, int32(ib.GetWidth()), int32(ib.GetHeight())); err != nil {
		return fmt.Errorf("cursor damage: %w", err)
	}
	if err := s.crs.surface.Commit(); err != nil {
		return fmt.Errorf("cursor commit: %w", err)
	}
	if err := s.pointer.SetCursor(s.pointerEnterSerial, s.crs.surface, int32(hx), int32(hy)); err != nil {
		return fmt.Errorf("set cursor: %w", err)
	}
	return nil
}

// fileCursorImage reads the xcursor file and picks its widest image.
func fileCursorImage(path string) (*xcursor.Image, error) {
	buf, err := os.ReadFile(path) //nolint:gosec // the path is the stylesheet's own url() cursor
	if err != nil {
		return nil, err
	}
	imgs, err := xcursor.ParseXcursor(buf)
	if err != nil {
		return nil, err
	}
	if len(imgs) == 0 {
		return nil, errors.New("no images")
	}
	img := imgs[0]
	for _, cand := range imgs[1:] {
		if cand.Width > img.Width {
			img = cand
		}
	}
	return img, nil
}

// fallbackFileCursorLocked drops a file cursor that cannot load back
// to the arrow, the theme path's own degradation.
func (s *Session) fallbackFileCursorLocked() error {
	c := &s.crs
	c.desired = ""
	cur, name, err := s.themeCursor("")
	if err != nil {
		return err
	}
	c.shown, c.cur = name, cur
	img := cur.GetCursorImage(0)
	return s.pushCursorLocked(img)
}
