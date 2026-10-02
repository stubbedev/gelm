// Package highlight holds syntax highlighters for widget.TextArea
// (GtkSourceView's language definitions). Each styles spans with
// GtkSourceView's def: style ids, so one widget.TextScheme colors
// every language.
package highlight

import (
	"regexp"
	"unicode"

	"github.com/stubbedev/gelm/widget"
)

// TOML highlights TOML as GtkSourceView's toml.lang does: comments,
// table headers (def:keyword), keys (def:type), strings with their
// escapes (def:special-char), booleans, integers (def:decimal), floats
// with inf and nan (def:floating-point), datetimes (def:constant), and
// anything else in a value as def:error. Multi-line strings, and
// arrays spanning lines, carry over through the line state.
type TOML struct{}

var _ widget.Highlighter = TOML{}

// TOML style ids.
const (
	tomlComment  = "def:comment"
	tomlTable    = "def:keyword"
	tomlKey      = "def:type"
	tomlString   = "def:string"
	tomlEscape   = "def:special-char"
	tomlBoolean  = "def:boolean"
	tomlInteger  = "def:decimal"
	tomlFloat    = "def:floating-point"
	tomlDatetime = "def:constant"
	tomlError    = "def:error"
)

// The line state: the multi-line string mode, then the open arrays
// and inline tables (one bit each, set for an inline table) with
// their depth, and whether the innermost inline table waits for a key.
const (
	modeNone    = 0
	modeBasic   = 1 // inside """
	modeLiteral = 2 // inside '''
	modeMask    = 3
	depthShift  = 2
	depthMask   = 31
	keyBit      = 1 << 7
	stackShift  = 8
	maxDepth    = 22
)

// tomlState is the decoded line state.
type tomlState struct {
	mode  int
	stack []bool // true: an inline table, false: an array
	key   bool   // the innermost inline table expects a key
}

func decodeState(s int) tomlState {
	st := tomlState{mode: s & modeMask, key: s&keyBit != 0}
	depth := (s >> depthShift) & depthMask
	for i := range depth {
		st.stack = append(st.stack, s&(1<<(stackShift+i)) != 0)
	}
	return st
}

func (st tomlState) encode() int {
	s := st.mode
	if st.key {
		s |= keyBit
	}
	depth := min(len(st.stack), maxDepth)
	s |= depth << depthShift
	for i := range depth {
		if st.stack[i] {
			s |= 1 << (stackShift + i)
		}
	}
	return s
}

var (
	tomlDatetimeRe = regexp.MustCompile(`^(?:\d{4}-\d\d-\d\d(?:[T ]\d\d:\d\d(?::\d\d(?:\.\d+)?)?)?(?:Z|[+-]\d\d:\d\d)?|\d\d:\d\d(?::\d\d(?:\.\d+)?)?)`)
	tomlHexRe      = regexp.MustCompile(`^0x[0-9a-fA-F][_0-9a-fA-F]*|^0o[0-7][_0-7]*|^0b[01][_01]*`)
	tomlNumberRe   = regexp.MustCompile(`^[+-]?(?:inf|nan)\b|^[+-]?(?:[1-9][0-9_]*|0)(?:\.[0-9_]+)?(?:[eE][+-]?[0-9_]+)?`)
	tomlIntegerRe  = regexp.MustCompile(`^[+-]?(?:[1-9][0-9_]*|0)$`)
)

// Highlight implements widget.Highlighter.
func (TOML) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	l := &tomlLine{rs: line, st: decodeState(state)}
	l.run()
	return l.spans, l.st.encode()
}

// tomlLine is one line's scan.
type tomlLine struct {
	rs    []rune
	i     int
	st    tomlState
	spans []widget.TextSpan
}

func (l *tomlLine) span(start, end int, class string) {
	if end > start {
		l.spans = append(l.spans, widget.TextSpan{Start: start, End: end, Class: class})
	}
}

func (l *tomlLine) skipSpace() {
	for l.i < len(l.rs) && (l.rs[l.i] == ' ' || l.rs[l.i] == '\t') {
		l.i++
	}
}

func (l *tomlLine) at(s string) bool {
	rs := []rune(s)
	if l.i+len(rs) > len(l.rs) {
		return false
	}
	for k, r := range rs {
		if l.rs[l.i+k] != r {
			return false
		}
	}
	return true
}

func (l *tomlLine) run() {
	if l.st.mode != modeNone {
		if !l.multiline(l.st.mode) {
			return
		}
	}
	if len(l.st.stack) == 0 {
		l.skipSpace()
		switch {
		case l.i >= len(l.rs):
			return
		case l.rs[l.i] == '#':
			l.comment()
			return
		case l.at("[[") || l.rs[l.i] == '[':
			l.header()
			l.tail()
			return
		}
		if !l.keys('=') {
			return
		}
	}
	l.values()
}

// comment styles the rest of the line.
func (l *tomlLine) comment() {
	l.span(l.i, len(l.rs), tomlComment)
	l.i = len(l.rs)
}

// header styles a [table] or [[array table]] header.
func (l *tomlLine) header() {
	start := l.i
	closing := "]"
	if l.at("[[") {
		closing = "]]"
	}
	for l.i < len(l.rs) && !l.at(closing) {
		l.i++
	}
	if l.i < len(l.rs) {
		l.i += len(closing)
	}
	l.span(start, l.i, tomlTable)
}

// tail is what may follow a header: a comment, else an error.
func (l *tomlLine) tail() {
	l.skipSpace()
	if l.i < len(l.rs) && l.rs[l.i] == '#' {
		l.comment()
		return
	}
	l.span(l.i, len(l.rs), tomlError)
	l.i = len(l.rs)
}

// keys styles a dotted key up to the '=' that ends it, and steps past
// it; false when the line ended (or a comment began) first.
func (l *tomlLine) keys(end rune) bool {
	for l.i < len(l.rs) {
		r := l.rs[l.i]
		switch {
		case r == ' ' || r == '\t' || r == '.':
			l.i++
		case r == end:
			l.i++
			return true
		case r == '#':
			l.comment()
			return false
		case r == '"' || r == '\'':
			start := l.i
			l.i++
			for l.i < len(l.rs) && l.rs[l.i] != r {
				if r == '"' && l.rs[l.i] == '\\' {
					l.i++
				}
				l.i++
			}
			l.i = min(l.i+1, len(l.rs))
			l.span(start, l.i, tomlKey)
		case isBareKey(r):
			start := l.i
			for l.i < len(l.rs) && isBareKey(l.rs[l.i]) {
				l.i++
			}
			l.span(start, l.i, tomlKey)
		default:
			l.span(l.i, l.i+1, tomlError)
			l.i++
		}
	}
	return false
}

func isBareKey(r rune) bool {
	return r == '_' || r == '-' || r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// values styles values to the end of the line, tracking the arrays and
// inline tables they open.
func (l *tomlLine) values() {
	for l.i < len(l.rs) {
		if n := len(l.st.stack); n > 0 && l.st.stack[n-1] && l.st.key {
			l.skipSpace()
			if l.i >= len(l.rs) {
				return
			}
			if l.rs[l.i] == '}' {
				l.close()
				continue
			}
			if !l.keys('=') {
				return
			}
			l.st.key = false
			continue
		}
		l.skipSpace()
		if l.i >= len(l.rs) {
			return
		}
		r := l.rs[l.i]
		switch {
		case r == '#':
			l.comment()
		case r == '[':
			l.st.stack = append(l.st.stack, false)
			l.i++
		case r == '{':
			l.st.stack = append(l.st.stack, true)
			l.st.key = true
			l.i++
		case r == ']' || r == '}':
			l.close()
		case r == ',':
			l.i++
			if n := len(l.st.stack); n > 0 && l.st.stack[n-1] {
				l.st.key = true
			}
		case l.at(`"""`):
			l.i += 3
			if !l.multiline(modeBasic) {
				return
			}
		case l.at(`'''`):
			l.i += 3
			if !l.multiline(modeLiteral) {
				return
			}
		case r == '"':
			l.basic()
		case r == '\'':
			start := l.i
			l.i++
			for l.i < len(l.rs) && l.rs[l.i] != '\'' {
				l.i++
			}
			l.i = min(l.i+1, len(l.rs))
			l.span(start, l.i, tomlString)
		default:
			l.scalar()
		}
	}
}

// close ends the innermost array or inline table.
func (l *tomlLine) close() {
	l.i++
	if n := len(l.st.stack); n > 0 {
		l.st.stack = l.st.stack[:n-1]
		l.st.key = false
	}
}

// basic styles a "basic string", its escapes apart.
func (l *tomlLine) basic() {
	start := l.i
	l.i++
	from := start
	for l.i < len(l.rs) && l.rs[l.i] != '"' {
		if l.rs[l.i] == '\\' {
			l.span(from, l.i, tomlString)
			esc := l.i
			l.escape()
			l.span(esc, l.i, tomlEscape)
			from = l.i
			continue
		}
		l.i++
	}
	l.i = min(l.i+1, len(l.rs))
	l.span(from, l.i, tomlString)
}

// escape steps over one backslash escape.
func (l *tomlLine) escape() {
	l.i++
	if l.i >= len(l.rs) {
		return
	}
	switch l.rs[l.i] {
	case 'u':
		l.i = min(l.i+5, len(l.rs))
	case 'U':
		l.i = min(l.i+9, len(l.rs))
	default:
		l.i++
	}
}

// multiline styles a multi-line string from l.i to its closing
// delimiter, true when it closed on this line (the state is clear),
// false when it runs on (the state holds mode).
func (l *tomlLine) multiline(mode int) bool {
	delim := `"""`
	if mode == modeLiteral {
		delim = `'''`
	}
	from := l.i
	if from >= 3 && mode != l.st.mode {
		from -= 3 // the opening delimiter
	}
	for l.i < len(l.rs) {
		if l.at(delim) {
			l.i += 3
			// A quote or two right after the delimiter belongs to the
			// string (TOML allows them before the closing three).
			for l.i < len(l.rs) && string(l.rs[l.i]) == delim[:1] {
				l.i++
			}
			l.span(from, l.i, tomlString)
			l.st.mode = modeNone
			return true
		}
		if mode == modeBasic && l.rs[l.i] == '\\' {
			l.span(from, l.i, tomlString)
			esc := l.i
			l.escape()
			l.span(esc, l.i, tomlEscape)
			from = l.i
			continue
		}
		l.i++
	}
	l.span(from, l.i, tomlString)
	l.st.mode = mode
	return false
}

// scalar styles a boolean, datetime, number or, failing all, one
// error rune.
func (l *tomlLine) scalar() {
	rest := string(l.rs[l.i:])
	word := func(w string) bool {
		if !l.at(w) {
			return false
		}
		end := l.i + len(w)
		return end == len(l.rs) || !isBareKey(l.rs[end])
	}
	match := func(re *regexp.Regexp) int {
		if loc := re.FindStringIndex(rest); loc != nil {
			return len([]rune(rest[:loc[1]]))
		}
		return 0
	}
	switch {
	case word("true") || word("false"):
		n := 4
		if l.rs[l.i] == 'f' {
			n = 5
		}
		l.span(l.i, l.i+n, tomlBoolean)
		l.i += n
	case match(tomlDatetimeRe) > 0:
		n := match(tomlDatetimeRe)
		l.span(l.i, l.i+n, tomlDatetime)
		l.i += n
	case match(tomlHexRe) > 0:
		n := match(tomlHexRe)
		l.span(l.i, l.i+n, tomlInteger)
		l.i += n
	case match(tomlNumberRe) > 0:
		n := match(tomlNumberRe)
		class := tomlFloat
		if tomlIntegerRe.MatchString(string(l.rs[l.i : l.i+n])) {
			class = tomlInteger
		}
		l.span(l.i, l.i+n, class)
		l.i += n
	default:
		l.span(l.i, l.i+1, tomlError)
		l.i++
	}
}
