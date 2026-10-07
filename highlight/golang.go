package highlight

import (
	"regexp"

	"github.com/stubbedev/gelm/widget"
)

// Go highlights Go as GtkSourceView's go.lang does: line and block
// comments (//go: directives and // +build lines as def:preprocessor),
// keywords, predeclared types, constants (true and false as booleans)
// and builtin functions, interpreted strings with their escapes, raw
// strings, runes, and decimal, base-n, floating-point and imaginary
// numbers. Block comments and raw strings carry over through the line
// state.
type Go struct{}

var _ widget.Highlighter = Go{}

// The Go line states.
const (
	goNone = iota
	goBlockComment
	goRawString
)

var (
	goKeywords = newSet("break", "case", "chan", "const", "continue", "default", "defer", "else",
		"fallthrough", "for", "func", "go", "goto", "if", "import", "interface", "map", "package",
		"range", "return", "select", "struct", "switch", "type", "var")
	goTypes = newSet("any", "bool", "byte", "comparable", "complex64", "complex128", "error",
		"float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr")
	goConstants = newSet("iota", "nil")
	goBooleans  = newSet("true", "false")
	goBuiltins  = newSet("append", "cap", "clear", "close", "complex", "copy", "delete", "imag",
		"len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover")

	// goNumbers are the numeric literals, the longest form first.
	goNumbers = []form{
		{regexp.MustCompile(`^(?:\d[\d_]*(?:\.[\d_]*)?(?:[eE][+-]?\d+)?|\.\d[\d_]*(?:[eE][+-]?\d+)?)i`), styleComplex},
		{regexp.MustCompile(`^0[xX][\da-fA-F_]+(?:\.[\da-fA-F_]*)?(?:[pP][+-]?\d+)?|^0[oO]?[0-7_]+\b|^0[bB][01_]+`), styleBaseN},
		{regexp.MustCompile(`^(?:\d[\d_]*\.[\d_]*(?:[eE][+-]?\d+)?|\d[\d_]*[eE][+-]?\d+|\.\d[\d_]*(?:[eE][+-]?\d+)?)`), styleFloat},
		{regexp.MustCompile(`^\d[\d_]*`), styleDecimal},
	}
)

// Highlight implements widget.Highlighter.
func (Go) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	switch state {
	case goBlockComment:
		if !s.until("*/", styleComment) {
			return s.spans, goBlockComment
		}
	case goRawString:
		if !s.until("`", styleString) {
			return s.spans, goRawString
		}
	}
	for !s.done() {
		r := s.rs[s.i]
		switch {
		case s.at("//go:") || s.at("// +build"):
			s.rest(stylePreproc)
		case s.at("//"):
			s.rest(styleComment)
		case s.at("/*"):
			if !s.delimited("/*", "*/", styleComment) {
				return s.spans, goBlockComment
			}
		case r == '`':
			if !s.delimited("`", "`", styleString) {
				return s.spans, goRawString
			}
		case r == '"':
			s.quoted('"', true, true, styleString)
		case r == '\'':
			s.quoted('\'', true, true, styleChar)
		case r >= '0' && r <= '9' || r == '.' && s.peek(1) >= '0' && s.peek(1) <= '9':
			if !s.first(goNumbers) {
				s.i++
			}
		case isIdent(r):
			start := s.i
			w := s.word(isIdent)
			switch {
			case goKeywords[w]:
				s.span(start, s.i, styleKeyword)
			case goTypes[w]:
				s.span(start, s.i, styleType)
			case goBooleans[w]:
				s.span(start, s.i, styleBoolean)
			case goConstants[w]:
				s.span(start, s.i, styleSpecialConst)
			case goBuiltins[w] && s.peek(0) == '(':
				s.span(start, s.i, styleBuiltin)
			}
		default:
			s.i++
		}
	}
	return s.spans, goNone
}
