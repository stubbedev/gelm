package widget

// Base is the embeddable shared state for widgets defined outside the
// kit: it carries the arranged bounds, parent link, invalidation flags,
// and measure cache the frame loop leans on, exactly what internal
// widgets get from node. Embed it, implement the Widget interface, and
// route leaf hit tests through HitLeaf.
type Base struct {
	node
}

// NewBase returns a zero Base for embedding:
//
//	type ruler struct {
//	    widget.Base
//	    ...
//	}
func NewBase() Base { return Base{} }

// SetParents records parent as the arranging container of every child,
// so containers defined outside the kit wire the cascade the same way
// the built-in ones do during Arrange.
func SetParents(parent Widget, kids ...Widget) { setParents(parent, kids...) }
