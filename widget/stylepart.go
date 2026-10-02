package widget

import "github.com/stubbedev/gelm/render"

// stylePart is a style-only node of a widget: matched and painted by
// its owning widget, never laid out or hit on its own. Every sub-node
// a GTK stylesheet names embeds it — the entry's `text`, the scale's
// `trough`, the switch's `slider`, the checkbutton's `check` — so one
// type carries the tree shape and the cascade resolves per part.
type stylePart struct{ node }

func (*stylePart) Measure(con Constraints) Size { return clampSize(Size{}, con) }
func (*stylePart) Paint(*render.Canvas)         {}
func (*stylePart) HitTest(Point) Widget         { return nil }
