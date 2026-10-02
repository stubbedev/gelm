// Global shortcuts over hyprland-global-shortcuts-v1: actions the
// compositor binds to keys and reports back, for apps with no focused
// window (a portal's GlobalShortcuts, a push-to-talk daemon).
package app

import (
	"errors"

	"github.com/stubbedev/gelm/wlr"
)

// ErrGlobalShortcutsUnavailable reports a compositor without
// hyprland_global_shortcuts_manager_v1.
var ErrGlobalShortcutsUnavailable = errors.New("app: compositor has no hyprland_global_shortcuts_manager_v1")

// GlobalShortcut is one registered shortcut.
type GlobalShortcut struct {
	p  *wlr.GlobalShortcutV1
	on func(pressed bool, unixSec uint64)
}

// GlobalShortcutsAvailable reports whether the compositor offers the
// protocol.
func (a *Application) GlobalShortcutsAvailable() bool {
	return a.sess.GlobalShortcutsManager() != nil
}

// RegisterGlobalShortcut registers id for appID; the compositor shows
// description and binds trigger (a user-readable hint, as the
// compositor's config decides the keys). on runs on the loop goroutine
// for every press (true) and release, with the event's Unix seconds.
// The app_id + id pair must be unique for the connection: a duplicate
// is the compositor's already_taken protocol error, so keep one
// registration per pair. Call on the loop goroutine.
func (a *Application) RegisterGlobalShortcut(id, appID, description, trigger string, on func(pressed bool, unixSec uint64)) (*GlobalShortcut, error) {
	mgr := a.sess.GlobalShortcutsManager()
	if mgr == nil {
		return nil, ErrGlobalShortcutsUnavailable
	}
	p, err := mgr.RegisterShortcut(id, appID, description, trigger)
	if err != nil {
		return nil, err
	}
	s := &GlobalShortcut{p: p, on: on}
	p.AddPressedHandler(s)
	p.AddReleasedHandler(s)
	return s, nil
}

// Destroy unregisters the shortcut. Call on the loop goroutine.
func (s *GlobalShortcut) Destroy() {
	if s.p == nil {
		return
	}
	_ = s.p.Destroy()
	s.p = nil
}

// HandleGlobalShortcutV1Pressed implements the protocol listener.
func (s *GlobalShortcut) HandleGlobalShortcutV1Pressed(ev wlr.GlobalShortcutV1PressedEvent) {
	s.on(true, shortcutSeconds(ev.TvSecHi, ev.TvSecLo))
}

// HandleGlobalShortcutV1Released implements the protocol listener.
func (s *GlobalShortcut) HandleGlobalShortcutV1Released(ev wlr.GlobalShortcutV1ReleasedEvent) {
	s.on(false, shortcutSeconds(ev.TvSecHi, ev.TvSecLo))
}

func shortcutSeconds(hi, lo uint32) uint64 { return uint64(hi)<<32 | uint64(lo) }
