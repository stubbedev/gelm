package highlight

import (
	"regexp"

	"github.com/stubbedev/gelm/widget"
)

// JSON highlights JSON as GtkSourceView's json.lang does: object keys
// (def:type, the TOML keys' style), strings with their escapes,
// numbers, true and false, null, and anything that is not JSON as
// def:error. JSON strings never span lines, so the state is always 0.
type JSON struct{}

var _ widget.Highlighter = JSON{}

var jsonNumbers = []form{
	{regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+(?:[eE][+-]?\d+)?|[eE][+-]?\d+)`), styleFloat},
	{regexp.MustCompile(`^-?(?:0|[1-9]\d*)`), styleDecimal},
}

// Highlight implements widget.Highlighter.
func (JSON) Highlight(line []rune, _ int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	for !s.done() {
		switch r := s.rs[s.i]; {
		case r == ' ' || r == '\t' || r == ',' || r == ':' || r == '[' || r == ']' || r == '{' || r == '}':
			s.i++
		case r == '"':
			// A key styles whole as the key; a value as a string with
			// its escapes apart.
			start := s.i
			s.skipQuoted('"', true)
			if s.keyFollows() {
				s.span(start, s.i, styleType)
			} else {
				s.i = start
				s.quoted('"', true, true, styleString)
			}
		case s.wordAt("true", isIdent) || s.wordAt("false", isIdent):
			start := s.i
			s.word(isIdent)
			s.span(start, s.i, styleBoolean)
		case s.wordAt("null", isIdent):
			s.take(4, styleSpecialConst)
		case !s.first(jsonNumbers):
			s.take(1, styleError)
		}
	}
	return s.spans, 0
}

// keyFollows reports whether a ':' is next past spaces - the string
// just scanned was an object key.
func (s *scanner) keyFollows() bool {
	for j := s.i; j < len(s.rs); j++ {
		switch s.rs[j] {
		case ' ', '\t':
			continue
		case ':':
			return true
		}
		return false
	}
	return false
}
