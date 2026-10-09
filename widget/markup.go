// Rich-label markup: a strict allowlist subset of HTML parsed into
// styled runs. ParseMarkup is total - it never panics on any input -
// and anything outside the allowlist fails the parse, which callers
// handle by rendering the input literally (RichLabel does).
package widget

import (
	"strings"

	"github.com/stubbedev/gelm/render"
)

// TextStyle is the style state markup tags accumulate over a run of
// text. The zero value is the plain label style.
type TextStyle struct {
	// Bold and Italic select the face variant a run shapes with.
	Bold, Italic bool
	// Underline and Strikethrough draw the decoration lines over the
	// run (Pango's <u> and <s>), in the run's color.
	Underline, Strikethrough bool
	// Color overrides the label color while non-zero. Premultiplied,
	// like every render.Color.
	Color render.Color
	// Href is the link target of a run; empty for plain text.
	Href string
}

// MarkupRun is a maximal stretch of text sharing one TextStyle.
type MarkupRun struct {
	// Text is the decoded plain UTF-8 text of the run. It never
	// contains '<' or an undecoded entity.
	Text string
	// Style is the style the run paints with.
	Style TextStyle
}

// ParseMarkup parses markup into styled runs. The accepted grammar is
// a strict allowlist, not HTML:
//
//	<b>bold</b>  <i>italic</i>  <u>underlined</u>  <s>struck</s>
//	<span color="#rrggbb">…</span>
//	<a href="https://example.com">link</a>
//
// Tags nest and combine (bold italic, colors inheriting into nested
// tags). Attribute values are single- or double-quoted. Color values
// are #rgb, #rrggbb, or #rrggbbaa hex. Anything else - an unknown tag
// or attribute, a stray or mismatched close tag, an unclosed tag,
// self-closing syntax, uppercase tags - fails the parse.
//
// Text and attribute values are escaped HTML-style: write &lt; &gt;
// &amp; &quot; &apos; for the literal characters. A bare '&' or '<',
// a numeric reference (&#38;), and unknown names (&nbsp;) are outside
// the allowlist and fail the parse.
//
// ParseMarkup never panics. When it reports false, callers render the
// input literally; RichLabel does exactly that.
func ParseMarkup(markup string) (runs []MarkupRun, ok bool) {
	p := markupParser{src: markup}
	if !p.parse() {
		return nil, false
	}
	return p.runs, true
}

// entities are the only character references the allowlist accepts.
var entities = map[string]byte{
	"amp":  '&',
	"lt":   '<',
	"gt":   '>',
	"quot": '"',
	"apos": '\'',
}

// markupParser walks the input once, accumulating plain text under the
// style the open tags imply.
type markupParser struct {
	src   string
	pos   int
	runs  []MarkupRun
	style TextStyle
	// stack holds, per open tag, the tag name and the style to
	// restore when it closes.
	stack []markupFrame
	text  strings.Builder
}

// markupFrame is one open tag on the parser's stack.
type markupFrame struct {
	name  string
	style TextStyle
}

// parse consumes the whole input; false means malformed.
func (p *markupParser) parse() bool {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '<':
			if !p.tag() {
				return false
			}
		case '&':
			if !p.entity(&p.text) {
				return false
			}
		default:
			p.text.WriteByte(p.src[p.pos])
			p.pos++
		}
	}
	if len(p.stack) != 0 {
		return false
	}
	p.flush()
	return true
}

// flush moves accumulated text into a run under the current style.
func (p *markupParser) flush() {
	if p.text.Len() == 0 {
		return
	}
	p.runs = append(p.runs, MarkupRun{Text: p.text.String(), Style: p.style})
	p.text.Reset()
}

// tag consumes one allowlisted tag starting at '<'. Every construct the
// allowlist does not name fails the parse.
func (p *markupParser) tag() bool {
	p.pos++
	if p.pos >= len(p.src) {
		return false
	}
	if p.src[p.pos] == '/' {
		return p.closeTag()
	}
	name, ok := p.name()
	if !ok {
		return false
	}
	switch name {
	case "b", "i", "u", "s":
		if !p.endTag() {
			return false
		}
		p.flush()
		p.stack = append(p.stack, markupFrame{name, p.style})
		switch name {
		case "b":
			p.style.Bold = true
		case "i":
			p.style.Italic = true
		case "u":
			p.style.Underline = true
		default:
			p.style.Strikethrough = true
		}
	case "span":
		return p.spanTag()
	case "a":
		return p.aTag()
	default:
		return false
	}
	return true
}

// spanTag parses <span> with an optional color attribute.
func (p *markupParser) spanTag() bool {
	p.flush()
	frame := markupFrame{"span", p.style}
	st := p.style
	for {
		p.spaces()
		c, ok := p.peek()
		if !ok {
			return false
		}
		if c == '>' {
			p.pos++
			break
		}
		attr, ok := p.name()
		if !ok || attr != "color" || !p.eq() {
			return false
		}
		v, ok := p.quoted()
		if !ok {
			return false
		}
		col, ok := parseHexColor(v)
		if !ok {
			return false
		}
		st.Color = col
	}
	p.stack = append(p.stack, frame)
	p.style = st
	return true
}

// aTag parses <a>, whose href attribute is required and non-empty.
func (p *markupParser) aTag() bool {
	p.flush()
	frame := markupFrame{"a", p.style}
	st := p.style
	href := ""
	seen := false
	for {
		p.spaces()
		c, ok := p.peek()
		if !ok {
			return false
		}
		if c == '>' {
			p.pos++
			break
		}
		attr, ok := p.name()
		if !ok || attr != "href" || !p.eq() {
			return false
		}
		v, ok := p.quoted()
		if !ok || v == "" {
			return false
		}
		href, seen = v, true
	}
	if !seen {
		return false
	}
	st.Href = href
	p.stack = append(p.stack, frame)
	p.style = st
	return true
}

// closeTag parses </name> and pops the matching open tag.
func (p *markupParser) closeTag() bool {
	p.pos++
	name, ok := p.name()
	if !ok || !p.endTag() {
		return false
	}
	if len(p.stack) == 0 || p.stack[len(p.stack)-1].name != name {
		return false
	}
	p.flush()
	p.style = p.stack[len(p.stack)-1].style
	p.stack = p.stack[:len(p.stack)-1]
	return true
}

// entity consumes "&name;" and writes the decoded byte to w. Numeric
// and unknown references fail the parse.
func (p *markupParser) entity(w *strings.Builder) bool {
	end := strings.IndexByte(p.src[p.pos:], ';')
	if end < 2 {
		return false
	}
	b, ok := entities[p.src[p.pos+1:p.pos+end]]
	if !ok {
		return false
	}
	w.WriteByte(b)
	p.pos += end + 1
	return true
}

// name reads a lowercase tag or attribute name.
func (p *markupParser) name() (string, bool) {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= 'a' && p.src[p.pos] <= 'z' {
		p.pos++
	}
	if p.pos == start {
		return "", false
	}
	return p.src[start:p.pos], true
}

// endTag consumes optional whitespace and the closing '>'.
func (p *markupParser) endTag() bool {
	p.spaces()
	c, ok := p.peek()
	if !ok || c != '>' {
		return false
	}
	p.pos++
	return true
}

// eq consumes optional whitespace, '=', and optional whitespace.
func (p *markupParser) eq() bool {
	p.spaces()
	c, ok := p.peek()
	if !ok || c != '=' {
		return false
	}
	p.pos++
	p.spaces()
	return true
}

// spaces consumes whitespace.
func (p *markupParser) spaces() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n') {
		p.pos++
	}
}

// peek returns the byte at the cursor, false at end of input.
func (p *markupParser) peek() (byte, bool) {
	if p.pos >= len(p.src) {
		return 0, false
	}
	return p.src[p.pos], true
}

// quoted reads a quoted attribute value, decoding entities inside it.
func (p *markupParser) quoted() (string, bool) {
	q, ok := p.peek()
	if !ok || (q != '"' && q != '\'') {
		return "", false
	}
	p.pos++
	var sb strings.Builder
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; c {
		case q:
			p.pos++
			return sb.String(), true
		case '<':
			// A raw '<' inside a value is markup, not text.
			return "", false
		case '&':
			if !p.entity(&sb) {
				return "", false
			}
		default:
			sb.WriteByte(c)
			p.pos++
		}
	}
	return "", false
}

// parseHexColor reads the allowlisted color forms into a premultiplied
// render.Color, expanding #rgb the way CSS does.
func parseHexColor(s string) (render.Color, bool) {
	if len(s) == 0 || s[0] != '#' {
		return 0, false
	}
	hex := s[1:]
	nib := func(c byte) (uint8, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		default:
			return 0, false
		}
	}
	switch len(hex) {
	case 3:
		r, ok1 := nib(hex[0])
		g, ok2 := nib(hex[1])
		b, ok3 := nib(hex[2])
		if !ok1 || !ok2 || !ok3 {
			return 0, false
		}
		return render.RGBA(r*0x11, g*0x11, b*0x11, 0xff), true
	case 6, 8:
		var c [4]uint8
		c[3] = 0xff
		for j := range len(hex) / 2 {
			hi, ok1 := nib(hex[j*2])
			lo, ok2 := nib(hex[j*2+1])
			if !ok1 || !ok2 {
				return 0, false
			}
			c[j] = hi<<4 | lo
		}
		return render.RGBA(c[0], c[1], c[2], c[3]), true
	default:
		return 0, false
	}
}
