package wlsession

import (
	"errors"
	"math"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/wlnull"
	"github.com/stubbedev/gelm/third_party/neurlang-wayland/wl"
	"github.com/stubbedev/gelm/wlr"
)

// Small per-surface protocols: the content-type hint (the compositor
// may tune for photos, video, games), the alpha modifier (opacity the
// compositor applies, no repaint), and the system bell. Each is
// optional: without it the setter reports ErrProtocolUnavailable and
// the surface renders as before.

// ErrProtocolUnavailable reports a compositor without the protocol a
// surface request needs.
var ErrProtocolUnavailable = errors.New("wlsession: protocol unavailable")

// ContentType is a surface's content kind (wp_content_type_v1).
type ContentType uint32

// Content types.
const (
	ContentNone  ContentType = wlr.WpContentTypeV1TypeNone
	ContentPhoto ContentType = wlr.WpContentTypeV1TypePhoto
	ContentVideo ContentType = wlr.WpContentTypeV1TypeVideo
	ContentGame  ContentType = wlr.WpContentTypeV1TypeGame
)

// bindSurfaceHint binds one of the per-surface hint globals.
func (s *Session) bindSurfaceHint(ev wl.RegistryGlobalEvent) {
	ctx, _ := wl.GetUserData[wl.Context](s.registry)
	switch ev.Interface {
	case "wp_content_type_manager_v1":
		mgr := wlr.NewWpContentTypeManagerV1(ctx)
		if s.bindOptional(ev, 1, mgr) {
			s.contentTypeMgr = mgr
		}
	case "wp_alpha_modifier_v1":
		mgr := wlr.NewWpAlphaModifierV1(ctx)
		if s.bindOptional(ev, 1, mgr) {
			s.alphaMgr = mgr
		}
	case "xdg_system_bell_v1":
		bell := wlr.NewXdgSystemBellV1(ctx)
		if s.bindOptional(ev, 1, bell) {
			s.bell = bell
		}
	default:
		return
	}
	debug.Log("shell", "%s bound", ev.Interface)
}

// SetContentType hints surf's content kind; it applies on the
// surface's next commit.
func (s *Session) SetContentType(surf *wl.Surface, ct ContentType) error {
	if s.contentTypeMgr == nil || surf == nil {
		return ErrProtocolUnavailable
	}
	obj := s.contentTypes[surf]
	if obj == nil {
		var err error
		if obj, err = s.contentTypeMgr.GetSurfaceType(surf); err != nil {
			return err
		}
		if s.contentTypes == nil {
			s.contentTypes = map[*wl.Surface]*wlr.WpContentTypeV1{}
		}
		s.contentTypes[surf] = obj
	}
	return obj.SetType(uint32(ct))
}

// SetSurfaceAlpha sets the opacity the compositor applies to surf
// (0..1) without the client repainting; it applies on the surface's
// next commit.
func (s *Session) SetSurfaceAlpha(surf *wl.Surface, alpha float64) error {
	if s.alphaMgr == nil || surf == nil {
		return ErrProtocolUnavailable
	}
	obj := s.alphaSurfaces[surf]
	if obj == nil {
		var err error
		if obj, err = s.alphaMgr.GetSurface(surf); err != nil {
			return err
		}
		if s.alphaSurfaces == nil {
			s.alphaSurfaces = map[*wl.Surface]*wlr.WpAlphaModifierSurfaceV1{}
		}
		s.alphaSurfaces[surf] = obj
	}
	alpha = math.Min(1, math.Max(0, alpha))
	return obj.SetMultiplier(uint32(math.Round(alpha * math.MaxUint32)))
}

// Bell rings the system bell for surf (nil: the session's), the error
// ding of a rejected input; a compositor may flash the window instead.
func (s *Session) Bell(surf *wl.Surface) error {
	if s.bell == nil {
		return ErrProtocolUnavailable
	}
	var target wl.Proxy = wlnull.Null
	if surf != nil {
		target = surf
	}
	return s.bell.Context().SendRequest(s.bell, 1, target)
}

// releaseSurfaceHints destroys surf's hint objects with its input.
func (s *Session) releaseSurfaceHints(surf *wl.Surface) {
	if obj := s.contentTypes[surf]; obj != nil {
		_ = obj.Destroy()
		delete(s.contentTypes, surf)
	}
	if obj := s.alphaSurfaces[surf]; obj != nil {
		_ = obj.Destroy()
		delete(s.alphaSurfaces, surf)
	}
}
