// Package layersurface maps the wlr-layer-shell role onto a wl_surface:
// anchor, layer, keyboard mode, the configure handshake, and the closed
// signal.
package layersurface

import (
	"errors"
	"fmt"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/wlr"
)

// ErrNotConfigured reports a draw attempt before the first configure
// event. The compositor rejects buffers committed before the initial
// configure.
var ErrNotConfigured = errors.New("layersurface: not configured yet")

// ErrClosed reports that the compositor closed the layer surface; the
// surface object must not be used again.
var ErrClosed = errors.New("layersurface: closed by compositor")

// Anchor is a bitmask of the edges the surface is anchored to.
type Anchor uint8

const (
	AnchorTop Anchor = 1 << iota
	AnchorBottom
	AnchorLeft
	AnchorRight
)

// Layer selects the stack layer the surface renders in. The numeric values
// are the wire values of the protocol enum.
type Layer uint8

const (
	LayerBackground Layer = iota
	LayerBottom
	LayerTop
	LayerOverlay
)

// KeyboardMode selects how the compositor gives the surface keyboard
// focus. The numeric values are the wire values of the protocol enum.
type KeyboardMode uint8

const (
	KeyboardNone KeyboardMode = iota
	KeyboardExclusive
	KeyboardOnDemand
)

// Config describes the layer surface before the first commit.
type Config struct {
	Layer  Layer
	Anchor Anchor

	// Width and Height are the surface size in surface (logical) pixels.
	// An axis set to 0 lets the compositor assign it, which the protocol
	// only allows when both edges of that axis are anchored; New
	// enforces this locally so the error surfaces before the wire.
	Width, Height uint32

	// ExclusiveZone reserves space along the anchored edge, in surface
	// pixels. Negative values distance the surface from the edge by the
	// given amount. 0 requests no reservation.
	ExclusiveZone int32

	Keyboard  KeyboardMode
	Namespace string
}

// Surface is one layer surface and its configure handshake.
type Surface struct {
	WLSurface *wl.Surface
	Layer     *wlr.ZwlrLayerSurfaceV1

	cfg        Config
	configured bool
	closed     bool
	width      uint32
	height     uint32
}

// New assigns the layer role and sends the initial state. The caller must
// still commit the wl_surface; the compositor answers with the first
// configure event.
func New(shell *wlr.ZwlrLayerShellV1, surf *wl.Surface, output *wl.Output, cfg Config) (*Surface, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ls, err := shell.GetLayerSurface(surf, output, uint32(cfg.Layer), cfg.Namespace)
	if err != nil {
		return nil, fmt.Errorf("layersurface: get_layer_surface: %w", err)
	}
	s := &Surface{WLSurface: surf, Layer: ls, cfg: cfg}
	ls.AddConfigureHandler(s)
	ls.AddClosedHandler(s)

	if err := ls.SetSize(cfg.Width, cfg.Height); err != nil {
		return nil, fmt.Errorf("layersurface: set_size: %w", err)
	}
	if err := ls.SetAnchor(uint32(cfg.Anchor)); err != nil {
		return nil, fmt.Errorf("layersurface: set_anchor: %w", err)
	}
	if err := ls.SetExclusiveZone(cfg.ExclusiveZone); err != nil {
		return nil, fmt.Errorf("layersurface: set_exclusive_zone: %w", err)
	}
	if err := ls.SetKeyboardInteractivity(uint32(cfg.Keyboard)); err != nil {
		return nil, fmt.Errorf("layersurface: set_keyboard_interactivity: %w", err)
	}
	return s, nil
}

// validate mirrors the protocol's invalid_size rule: an automatic (zero)
// axis is only allowed when both edges of that axis are anchored.
func (c Config) validate() error {
	if c.Width == 0 && c.Anchor&(AnchorLeft|AnchorRight) != AnchorLeft|AnchorRight {
		return fmt.Errorf("layersurface: auto width needs both left and right anchors (anchor=%d)", c.Anchor)
	}
	if c.Height == 0 && c.Anchor&(AnchorTop|AnchorBottom) != AnchorTop|AnchorBottom {
		return fmt.Errorf("layersurface: auto height needs both top and bottom anchors (anchor=%d)", c.Anchor)
	}
	return nil
}

// HandleZwlrLayerSurfaceV1Configure implements the configure handler: it
// records the new size and acknowledges the serial. A zero width or height
// keeps the current choice for that axis, per the protocol.
func (s *Surface) HandleZwlrLayerSurfaceV1Configure(ev wlr.ZwlrLayerSurfaceV1ConfigureEvent) {
	s.applyConfigure(ev.Width, ev.Height)
	_ = s.Layer.AckConfigure(ev.Serial)
}

// applyConfigure records the configure without touching the wire; tests
// exercise the state machine through it.
func (s *Surface) applyConfigure(width, height uint32) {
	s.configured = true
	if width != 0 {
		s.width = width
	}
	if height != 0 {
		s.height = height
	}
}

// HandleZwlrLayerSurfaceV1Closed marks the surface closed.
func (s *Surface) HandleZwlrLayerSurfaceV1Closed(wlr.ZwlrLayerSurfaceV1ClosedEvent) {
	s.closed = true
}

// EnsureUsable gates drawing: the surface must have completed the first
// configure and must not be closed. A closed surface wins over the
// configured state.
func (s *Surface) EnsureUsable() error {
	if s.closed {
		return ErrClosed
	}
	if !s.configured {
		return ErrNotConfigured
	}
	return nil
}

// Closed reports whether the compositor closed the surface.
func (s *Surface) Closed() bool {
	return s.closed
}

// Size returns the last configured size in surface (logical) pixels. An
// unconfigured surface reports zeros.
func (s *Surface) Size() (int, int) {
	return int(s.width), int(s.height)
}

// HostSurface returns the underlying wl_surface.
func (s *Surface) HostSurface() *wl.Surface { return s.WLSurface }
