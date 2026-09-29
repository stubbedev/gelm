package app

import "github.com/stubbedev/gelm/internal/layersurface"

// Public aliases for the layer-surface enums. The internal package is
// importable only inside gelm, so external consumers of LayerConfig
// address layers, anchors, margins, and keyboard modes through these.

// Layer selects the stack layer a layer surface renders in.
type Layer = layersurface.Layer

// Stack layers, background through overlay.
const (
	LayerBackground = layersurface.LayerBackground
	LayerBottom     = layersurface.LayerBottom
	LayerTop        = layersurface.LayerTop
	LayerOverlay    = layersurface.LayerOverlay
)

// Anchor selects the anchored edges of a layer surface.
type Anchor = layersurface.Anchor

// Anchor edges.
const (
	AnchorTop    = layersurface.AnchorTop
	AnchorBottom = layersurface.AnchorBottom
	AnchorLeft   = layersurface.AnchorLeft
	AnchorRight  = layersurface.AnchorRight
)

// Margins is the space between a surface and the edges it anchors to,
// in logical pixels.
type Margins = layersurface.Margins

// KeyboardMode selects how the compositor gives a surface keyboard
// focus.
type KeyboardMode = layersurface.KeyboardMode

// Keyboard interactivity modes.
const (
	KeyboardNone      = layersurface.KeyboardNone
	KeyboardExclusive = layersurface.KeyboardExclusive
	KeyboardOnDemand  = layersurface.KeyboardOnDemand
)
