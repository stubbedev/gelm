package highlight

import (
	"regexp"
	"strings"

	"github.com/stubbedev/gelm/widget"
)

// YAML highlights YAML as GtkSourceView's yaml.lang does: comments,
// document markers (def:preprocessor), mapping keys (def:type), anchors
// and aliases (def:identifier), tags (def:preprocessor), quoted strings
// with their escapes, block scalars (| and >, their lines def:string
// while indented past the key that opened them), and plain scalars
// typed as booleans, null, or numbers. Block scalars and quoted strings
// spanning lines carry over through the line state.
type YAML struct{}

var _ widget.Highlighter = YAML{}

// The YAML line state: a mode in the low bits, and for a block scalar
// the indentation of the line that opened it.
const (
	yamlNone = iota
	yamlBlock
	yamlDouble
	yamlSingle
	yamlModeMask    = 3
	yamlIndentShift = 2
)

var (
	yamlBooleans = newSet("true", "false", "yes", "no", "on", "off")
	yamlNulls    = newSet("null", "~")
	yamlNumbers  = []form{
		{regexp.MustCompile(`^[-+]?(?:\d[\d_]*\.\d*(?:[eE][-+]?\d+)?|\.\d+(?:[eE][-+]?\d+)?|\d[\d_]*[eE][-+]?\d+|\.(?:inf|Inf|INF|nan|NaN|NAN))$`), styleFloat},
		{regexp.MustCompile(`^(?:[-+]?\d[\d_]*|0x[\da-fA-F_]+|0o[0-7_]+)$`), styleDecimal},
	}
	yamlBlockRe = regexp.MustCompile(`^[|>][-+0-9]*\s*(?:#.*)?$`)
)

// Highlight implements widget.Highlighter.
func (YAML) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	switch state & yamlModeMask {
	case yamlBlock:
		// Blank lines and lines indented past the opener are the
		// scalar's; anything else ends it.
		if open := state >> yamlIndentShift; indent == len(line) || indent > open {
			s.span(indent, len(line), styleString)
			return s.spans, state
		}
	case yamlDouble:
		if !s.quoted('"', false, true, styleString) {
			return s.spans, state
		}
		return s.spans, s.yamlValue(indent)
	case yamlSingle:
		if !s.quoted('\'', false, false, styleString) {
			return s.spans, state
		}
		return s.spans, s.yamlValue(indent)
	}
	s.i = indent
	switch {
	case s.done():
		return nil, yamlNone
	case s.rs[s.i] == '#':
		s.rest(styleComment)
		return s.spans, yamlNone
	case indent == 0 && (s.at("---") || s.at("...")) && (len(line) == 3 || line[3] == ' '):
		s.take(3, stylePreproc)
	}
	// Sequence entries ("- "), then a key, then its value.
	for {
		s.skipSpace()
		if s.peek(0) != '-' || (s.peek(1) != ' ' && s.peek(1) != 0) {
			break
		}
		s.i++
	}
	s.skipSpace()
	s.yamlKey()
	return s.spans, s.yamlValue(indent)
}

// yamlKey styles a mapping key at the cursor - plain or quoted, up to
// a ':' that ends the line or is followed by a space - and steps past
// the ':'; without one the cursor stays put.
func (s *scanner) yamlKey() {
	start := s.i
	j := s.i
	if r := s.peek(0); r == '"' || r == '\'' {
		s.skipQuoted(r, r == '"')
		j = s.i
		s.i = start
	} else {
		for j < len(s.rs) && s.rs[j] != ':' && s.rs[j] != '#' {
			j++
		}
	}
	if j >= len(s.rs) || s.rs[j] != ':' || (j+1 < len(s.rs) && s.rs[j+1] != ' ' && s.rs[j+1] != '\t') {
		return
	}
	end := j
	for end > start && (s.rs[end-1] == ' ' || s.rs[end-1] == '\t') {
		end--
	}
	s.span(start, end, styleType)
	s.i = j + 1
}

// yamlValue styles a value from the cursor to the end of the line and
// returns the state the line ends in (indent is the line's own, a
// block scalar's reference).
func (s *scanner) yamlValue(indent int) int {
	for !s.done() {
		s.skipSpace()
		if s.done() {
			break
		}
		switch r := s.rs[s.i]; {
		case r == '#' && (s.i == 0 || s.rs[s.i-1] == ' ' || s.rs[s.i-1] == '\t'):
			s.rest(styleComment)
		case r == '&' || r == '*':
			start := s.i
			s.i++
			s.word(func(r rune) bool { return r != ' ' && r != '\t' && r != ',' && r != ']' && r != '}' })
			s.span(start, s.i, styleIdentifier)
		case r == '!':
			start := s.i
			s.word(func(r rune) bool { return r != ' ' && r != '\t' })
			s.span(start, s.i, stylePreproc)
		case (r == '|' || r == '>') && s.match(yamlBlockRe) > 0:
			s.take(1, styleEscape)
			s.i = len(s.rs)
			return yamlBlock | indent<<yamlIndentShift
		case r == '"':
			if !s.quoted('"', true, true, styleString) {
				return yamlDouble
			}
		case r == '\'':
			if !s.quoted('\'', true, false, styleString) {
				return yamlSingle
			}
		case r == '[' || r == ']' || r == '{' || r == '}' || r == ',':
			s.i++
		default:
			s.yamlPlain()
		}
	}
	return yamlNone
}

// yamlPlain types a plain scalar: up to a comment, a flow indicator,
// or a ': ' - which makes it a key (a flow mapping's) - a boolean,
// null, or number is styled; other text stays plain.
func (s *scanner) yamlPlain() {
	start := s.i
	key := false
	for s.i < len(s.rs) {
		r := s.rs[s.i]
		if r == ',' || r == ']' || r == '}' || (r == '#' && s.i > start && (s.rs[s.i-1] == ' ' || s.rs[s.i-1] == '\t')) {
			break
		}
		if r == ':' && strings.ContainsRune(" \t,]}", s.peek(1)) || r == ':' && s.i+1 == len(s.rs) {
			key = true
			break
		}
		s.i++
	}
	end := s.i
	for end > start && (s.rs[end-1] == ' ' || s.rs[end-1] == '\t') {
		end--
	}
	if key {
		s.span(start, end, styleType)
		s.i++ // the ':'
		return
	}
	word := string(s.rs[start:end])
	lower := strings.ToLower(word)
	switch {
	case yamlBooleans[lower]:
		s.span(start, end, styleBoolean)
	case yamlNulls[lower]:
		s.span(start, end, styleSpecialConst)
	default:
		for _, f := range yamlNumbers {
			if f.re.MatchString(word) {
				s.span(start, end, f.class)
				break
			}
		}
	}
}
