// Package gelm is a retained-mode, CSS-themed widget kit for Wayland,
// written in pure Go (no cgo).
//
// The root package will carry the public widget API from M2 on. Today the
// tree is:
//
//	wlr       - generated wlr-layer-shell-unstable-v1 bindings
//	render    - pure rectangle and damage algebra
//	internal  - session, buffer pool, and layer surface plumbing
//	cmd       - milestone demos, starting with the M0 bar
package gelm
