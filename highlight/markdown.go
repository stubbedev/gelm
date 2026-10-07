package highlight

import (
	"regexp"
	"unicode"

	"github.com/stubbedev/gelm/widget"
)

// Markdown highlights CommonMark as GtkSourceView's markdown.lang does:
// ATX headings, thematic breaks, blockquote and list markers, inline
// code, strong and plain emphasis, links and autolinks, and backslash
// escapes. A fenced code block's lines go to the language its info
// string names (```go, ~~~yaml - any registered language or alias),
// carrying that language's own line state inside Markdown's; an
// unknown or missing hint styles the block preformatted.
type Markdown struct{}

var _ widget.Highlighter = Markdown{}

// The Markdown line state: the fence flag, its character (tilde) and
// length, the fenced language's registry id, and that language's own
// state above them.
const (
	mdFence      = 1 << 0
	mdTilde      = 1 << 1
	mdLenShift   = 2
	mdLenMask    = 15
	mdLangShift  = 6
	mdLangMask   = 255
	mdInnerShift = 14
)

var (
	mdFenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})\\s*([^`\\s]*)")
	mdHeadingRe = regexp.MustCompile(`^ {0,3}#{1,6}(?:\s|$)`)
	mdBreakRe   = regexp.MustCompile(`^ {0,3}(?:(?:\*\s*){3,}|(?:-\s*){3,}|(?:_\s*){3,})$`)
	mdListRe    = regexp.MustCompile(`^(?:[-+*]|\d{1,9}[.)])(?:\s|$)`)
	mdAutoRe    = regexp.MustCompile(`^<(?:[a-zA-Z][a-zA-Z0-9+.-]{1,31}:[^\s<>]*|[^\s<>@]+@[^\s<>]+)>`)
)

// Highlight implements widget.Highlighter.
func (Markdown) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	if state&mdFence != 0 {
		return s.fenced(state)
	}
	text := string(line)
	if m := mdFenceRe.FindStringSubmatch(text); m != nil {
		s.rest(stylePreformatted)
		st := mdFence | min(len(m[1]), mdLenMask)<<mdLenShift | languageID(m[2])<<mdLangShift
		if m[1][0] == '~' {
			st |= mdTilde
		}
		return s.spans, st
	}
	switch {
	case mdHeadingRe.MatchString(text):
		s.rest(styleHeading)
		return s.spans, 0
	case mdBreakRe.MatchString(text):
		s.rest(styleBreak)
		return s.spans, 0
	}
	// Container markers: blockquote '>'s and a list marker, then text.
	for {
		s.skipSpace()
		if s.peek(0) != '>' {
			break
		}
		s.take(1, styleQuoteMarker)
	}
	if n := s.match(mdListRe); n > 0 {
		if r := s.rs[s.i+n-1]; r == ' ' || r == '\t' {
			n-- // the marker, not the space after it
		}
		s.take(n, styleListMarker)
	}
	s.inline()
	return s.spans, 0
}

// fenced styles a line inside a fenced block: the closing fence ends
// it, any other line goes to the block's language (or styles
// preformatted).
func (s *scanner) fenced(state int) ([]widget.TextSpan, int) {
	char := '`'
	if state&mdTilde != 0 {
		char = '~'
	}
	need := (state >> mdLenShift) & mdLenMask
	s.skipSpace()
	if s.i <= 3 {
		start, n := s.i, 0
		for s.i < len(s.rs) && s.rs[s.i] == char {
			s.i++
			n++
		}
		s.skipSpace()
		if n >= need && s.done() {
			s.span(start, s.i, stylePreformatted)
			return s.spans, 0
		}
	}
	s.i = 0
	h := languageAt((state >> mdLangShift) & mdLangMask)
	if h == nil {
		s.rest(stylePreformatted)
		return s.spans, state
	}
	spans, inner := h.Highlight(s.rs, state>>mdInnerShift)
	return spans, state&(1<<mdInnerShift-1) | inner<<mdInnerShift
}

// inline styles a paragraph line's inline constructs.
func (s *scanner) inline() {
	for !s.done() {
		r := s.rs[s.i]
		switch {
		case r == '\\' && (unicode.IsPunct(s.peek(1)) || unicode.IsSymbol(s.peek(1))):
			s.take(2, styleEscape)
		case r == '`':
			s.codeSpan()
		case r == '<' && s.match(mdAutoRe) > 0:
			s.take(s.match(mdAutoRe), styleLinkDest)
		case r == '[':
			s.link()
		case r == '*' || r == '_':
			s.emphasis(r)
		default:
			s.i++
		}
	}
}

// codeSpan styles a run of backticks through the matching run, or
// steps over the opening run when none closes it on the line.
func (s *scanner) codeSpan() {
	start := s.i
	n := len(s.word(func(r rune) bool { return r == '`' }))
	for j := s.i; j < len(s.rs); {
		if s.rs[j] != '`' {
			j++
			continue
		}
		k := j
		for k < len(s.rs) && s.rs[k] == '`' {
			k++
		}
		if k-j == n {
			s.span(start, k, styleInlineCode)
			s.i = k
			return
		}
		j = k
	}
}

// link styles [text](destination): the bracketed text, then the
// parenthesized destination; a bracket without both is plain.
func (s *scanner) link() {
	start := s.i
	depth := 0
	for j := s.i; j < len(s.rs); j++ {
		switch s.rs[j] {
		case '[':
			depth++
		case ']':
			depth--
			if depth > 0 {
				continue
			}
			if j+1 >= len(s.rs) || s.rs[j+1] != '(' {
				s.i++
				return
			}
			for k := j + 2; k < len(s.rs); k++ {
				if s.rs[k] == ')' {
					s.span(start, j+1, styleLinkText)
					s.span(j+1, k+1, styleLinkDest)
					s.i = k + 1
					return
				}
			}
			s.i++
			return
		}
	}
	s.i++
}

// emphasis styles **strong** (or __strong__) and *emphasis* (or
// _emphasis_) when the closing delimiter comes on the line; an
// underscore inside a word opens nothing.
func (s *scanner) emphasis(d rune) {
	if d == '_' && s.i > 0 && isIdent(s.rs[s.i-1]) {
		s.i++
		return
	}
	n := 1
	class := styleEmphasis
	if s.peek(1) == d {
		n, class = 2, styleStrong
	}
	if c := s.i + n; c >= len(s.rs) || s.rs[c] == ' ' {
		s.i += n // a delimiter followed by space opens nothing
		return
	}
	for j := s.i + n; j+n <= len(s.rs); j++ {
		if s.rs[j] != d || s.rs[j-1] == ' ' || (n == 2 && s.rs[j+1] != d) {
			continue
		}
		if n == 1 && j+1 < len(s.rs) && s.rs[j+1] == d {
			j++ // a strong delimiter inside plain emphasis
			continue
		}
		s.span(s.i, j+n, class)
		s.i = j + n
		return
	}
	s.i += n
}
