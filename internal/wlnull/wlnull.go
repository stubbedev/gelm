// Package wlnull is the null object argument the generated Wayland
// bindings cannot send: a typed-nil proxy (a nil *xdg.Surface, a nil
// *wl.Output) encodes as object 0 but then panics in the wire encoder's
// new-id scan, which calls Id on every proxy argument. Null is a nil
// pointer the wire writes as id 0 with every proxy method safe on it.
package wlnull

import "github.com/neurlang/wayland/wl"

// Object is the nil-safe null proxy type.
type Object struct{}

// Null is the null object argument.
var Null = (*Object)(nil)

func (*Object) Context() *wl.Context   { return nil }
func (*Object) SetContext(*wl.Context) {}
func (*Object) Id() wl.ProxyId         { return 0 }
func (*Object) SetId(wl.ProxyId)       {}
func (*Object) Unregister()            {}
