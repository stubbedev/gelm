// Debug plumbing shared by every widget: optional names plus the live
// tree snapshot the inspector's dump and overlay render. This is the
// always-available half of the inspector (see docs/inspector.md): the
// dump only reads state, so it ships in prod binaries; the app-level
// overlay and its keybindings live in internal/inspect and the app
// package, gated behind GELM_INSPECT or an explicit opt-in.
package widget

import (
	"fmt"
	"io"
	"strings"
)

// DebugNamer is implemented by widgets carrying a debug name set with
// SetDebugName. Every widget that embeds node has the method; the
// interface lets the inspector also recognize third-party Widget
// implementations that mirror the API.
type DebugNamer interface {
	// DebugName returns the debug name, empty when none is set.
	DebugName() string
}

// SetDebugName labels the widget for the inspector: the tree dump and
// the overlay show it, so bug reports can point at "the save button"
// instead of "*widget.Button #3". Names are plain data: nothing in the
// toolkit reads them, and text or state updates never touch them.
func (n *node) SetDebugName(name string) {
	n.debugName = name
}

// DebugName returns the widget's debug name, empty when none is set.
func (n *node) DebugName() string { return n.debugName }

// NodeInfo is one live snapshot row of the widget tree: identity
// (type, debug name), the accessibility summary (role, tooltip name,
// bounds, value), and the router-driven input state. Unlike the a11y
// snapshot (Describe), the flags here are the live input state, which
// is what a layout bug report needs.
type NodeInfo struct {
	A11yState
	// TypeName is the widget's Go type, as %T prints it.
	TypeName string
	// DebugName is the SetDebugName label, empty when unnamed.
	DebugName string
	// Depth is the nesting level; the root is 0.
	Depth int
	// Focused, Hovered, and Pressed report that this widget is the
	// router's current focus, hover, or press target. All false when
	// the snapshot was taken without a Router.
	Focused, Hovered, Pressed bool
}

// InspectTree snapshots root and every descendant in paint order with
// nesting depth, the live-state counterpart of DescribeTree. Router
// may be nil, which leaves the input flags false.
func InspectTree(root Widget, r *Router) []NodeInfo {
	var out []NodeInfo
	walkTree(root, 0, func(w Widget, depth int) {
		ni := NodeInfo{
			A11yState: Describe(w),
			TypeName:  fmt.Sprintf("%T", w),
			Depth:     depth,
		}
		if d, ok := w.(DebugNamer); ok {
			ni.DebugName = d.DebugName()
		}
		if r != nil {
			ni.Focused = r.Focused() == w
			ni.Hovered = r.Hovered() == w
			ni.Pressed = r.Pressed() == w
		}
		out = append(out, ni)
	})
	return out
}

// DumpTree renders the tree as one indented line per widget: type,
// debug name, bounds, role, tooltip, and the set input-state flags.
// The format is stable - tests pin it byte for byte - so a dump pasted
// into a bug report says exactly the same thing to everyone.
//
//	*widget.Box bounds=0,0+400x200
//	  *widget.Button name=save bounds=8,8+72x36 role=button tooltip="Save" focusable
//	    *widget.Label bounds=18,17+52x18 role=label focusable
//
// Flags read "this widget IS the router's X target": focused, hover,
// pressed. focusable marks Tab-reachable widgets. r may be nil.
func DumpTree(root Widget, r *Router) string {
	var b strings.Builder
	DumpTreeTo(&b, root, r)
	return b.String()
}

// DumpTreeTo writes the DumpTree format to w.
func DumpTreeTo(w io.Writer, root Widget, r *Router) {
	nodes := InspectTree(root, r)
	for i, ni := range nodes {
		if i > 0 {
			_, _ = io.WriteString(w, "\n")
		}
		_, _ = io.WriteString(w, ni.dumpLine())
	}
	_, _ = io.WriteString(w, "\n")
}

// dumpLine renders one NodeInfo in the DumpTree format. Field order is
// part of the pinned format: type, name, bounds, role, tooltip, then
// the flags in fixed order.
func (ni NodeInfo) dumpLine() string {
	var b strings.Builder
	for range ni.Depth {
		b.WriteString("  ")
	}
	b.WriteString(ni.TypeName)
	if ni.DebugName != "" {
		fmt.Fprintf(&b, " name=%q", ni.DebugName)
	}
	bs := ni.Bounds
	fmt.Fprintf(&b, " bounds=%d,%d+%dx%d", bs.X, bs.Y, bs.W, bs.H)
	if ni.Role != RoleNone {
		fmt.Fprintf(&b, " role=%s", ni.Role)
	}
	if ni.Name != "" {
		fmt.Fprintf(&b, " tooltip=%q", ni.Name)
	}
	if ni.Focusable {
		b.WriteString(" focusable")
	}
	if ni.Focused {
		b.WriteString(" focused")
	}
	if ni.Hovered {
		b.WriteString(" hover")
	}
	if ni.Pressed {
		b.WriteString(" pressed")
	}
	return b.String()
}
