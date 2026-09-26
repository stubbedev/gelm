// Package gelm is a retained-mode widget kit for Wayland, written in
// pure Go (no cgo).
//
// The public API lives in two packages:
//
//	app       - the parked event loop, windows (xdg toplevels and
//	              layer surfaces), popovers, dialogs, and input wiring
//	widget    - the retained widget tree: labels, buttons, entries,
//	              text areas, lists, notebooks, menus, icons, themes
//
// Supporting it:
//
//	render    - premultiplied-alpha canvas, shaped text, icons
//	wlr       - generated Wayland protocol bindings
//	internal  - session, buffer arena, scale, popup, and drag-drop
//	            plumbing
//	cmd       - the demos (gelm-hello showcase, gelm-multi, panel, bar)
//
// The docs/ directory carries the design contracts: architecture,
// input model, application model, accessibility decision, and icon
// theming.
package gelm
