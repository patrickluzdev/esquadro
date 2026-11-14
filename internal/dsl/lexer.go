package dsl

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

// Comment is a regular (non-doc) comment, kept so the formatter can
// reproduce it.
type Comment struct {
	Text string // including the "//" or "/* */" markers
	Pos  diag.Pos
	End  diag.Pos
	// After is the kind of the last token before the comment and AfterEnd
	// its end offset. A comment right after a node's last token, on the same
	// line, is that node's trailing comment.
	After    Kind
	AfterEnd int
}

type lexer struct {
	src      []byte
	file     string
	off      int
	line     int
	col      int
	toks     []Token
	comments []Comment
	prevKind Kind // kind and end offset of the last token, for comments
	prevEnd  int
	diags    *diag.List
}

// lex splits src into tokens. Lexical errors are added to diags and produce
// Illegal tokens, so the parser can keep going.
func lex(file string, src []byte, diags *diag.List) ([]Token, []Comment) {
	l := &lexer{src: src, file: file, line: 1, col: 1, diags: diags, prevKind: Newline}
	if strings.HasPrefix(string(src), "\xef\xbb\xbf") {
		l.off = 3
	}
	for {
		t := l.next()
		l.toks = append(l.toks, t)
		if t.Kind != DocComment {
			l.prevKind, l.prevEnd = t.Kind, t.End.Offset
		}
		if t.Kind == EOF {
			return l.toks, l.comments
		}
	}
}

func (l *lexer) pos() diag.Pos {
	return diag.Pos{File: l.file, Line: l.line, Col: l.col, Offset: l.off}
}

// peek returns the rune at the current offset plus n runes, or -1 at EOF.
func (l *lexer) peek(n int) rune {
	off := l.off
	for {
		if off >= len(l.src) {
			return -1
		}
		r, size := utf8.DecodeRune(l.src[off:])
		if n == 0 {
			return r
		}
		off += size
		n--
	}
}

func (l *lexer) advance() rune {
	r, size := utf8.DecodeRune(l.src[l.off:])
	l.off += size
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) tok(kind Kind, start diag.Pos) Token {
	return Token{Kind: kind, Text: string(l.src[start.Offset:l.off]), Pos: start, End: l.pos()}
}

func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isIdentPart(r rune) bool  { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
func isDigit(r rune) bool      { return r >= '0' && r <= '9' }

func (l *lexer) next() Token {
	for {
		r := l.peek(0)
		switch {
		case r == ' ' || r == '\t' || r == '\r':
			l.advance()
			continue
		case r == '/' && l.peek(1) == '/' && l.peek(2) == '/' && l.peek(3) != '/':
			return l.docComment()
		case r == '/' && l.peek(1) == '/':
			l.lineComment()
			continue
		case r == '/' && l.peek(1) == '*':
			l.blockComment()
			continue
		}
		break
	}

	start := l.pos()
	r := l.peek(0)
	switch {
	case r == -1:
		return Token{Kind: EOF, Pos: start, End: start}
	case r == '\n':
		l.advance()
		return l.tok(Newline, start)
	case isIdentStart(r):
		return l.ident(start)
	case isDigit(r) || (r == '-' && isDigit(l.peek(1))):
		return l.number(start)
	case r == '"':
		return l.str(start)
	case r == '`':
		return l.rawStr(start)
	case r == '-' && l.peek(1) == '>':
		l.advance()
		l.advance()
		return l.tok(Arrow, start)
	case r == '<' && l.peek(1) == '-' && l.peek(2) == '>':
		l.advance()
		l.advance()
		l.advance()
		return l.tok(BiArrow, start)
	}

	l.advance()
	single := map[rune]Kind{
		'{': LBrace, '}': RBrace, '(': LParen, ')': RParen,
		'[': LBrack, ']': RBrack, ',': Comma, '@': At,
	}
	if k, ok := single[r]; ok {
		return l.tok(k, start)
	}
	t := l.tok(Illegal, start)
	if r == utf8.RuneError {
		l.diags.Errorf(start, t.End, "invalid UTF-8 encoding")
	} else {
		l.diags.Errorf(start, t.End, "invalid character %q", r)
	}
	return t
}

// ident lexes a name. A "-" is allowed inside a name when followed by a letter
// or digit, so blueprint names like admin-htmx are a single token, while
// "a->b" still lexes as a, ->, b.
func (l *lexer) ident(start diag.Pos) Token {
	for {
		r := l.peek(0)
		if isIdentPart(r) {
			l.advance()
			continue
		}
		if r == '-' && isIdentPart(l.peek(1)) {
			l.advance()
			continue
		}
		break
	}
	return l.tok(Name, start)
}

func (l *lexer) number(start diag.Pos) Token {
	if l.peek(0) == '-' {
		l.advance()
	}
	for isDigit(l.peek(0)) {
		l.advance()
	}
	if l.peek(0) == '.' && isDigit(l.peek(1)) {
		l.advance()
		for isDigit(l.peek(0)) {
			l.advance()
		}
	}
	return l.tok(Number, start)
}

func (l *lexer) str(start diag.Pos) Token {
	l.advance() // opening quote
	var b strings.Builder
	for {
		r := l.peek(0)
		switch r {
		case -1, '\n':
			t := l.tok(Illegal, start)
			l.diags.Errorf(start, t.End, "unterminated string")
			return t
		case '"':
			l.advance()
			t := l.tok(String, start)
			t.Value = b.String()
			return t
		case '\\':
			escPos := l.pos()
			l.advance()
			e := l.peek(0)
			switch e {
			case '"', '\\':
				b.WriteRune(e)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case -1, '\n':
				continue // reported as unterminated string
			default:
				l.diags.Errorf(escPos, diag.Pos{}, `invalid escape sequence "\%c"`, e).Suggestion =
					"use a raw string with backticks to write backslashes literally, e.g. `\\d+`"
			}
			l.advance()
		default:
			b.WriteRune(l.advance())
		}
	}
}

func (l *lexer) rawStr(start diag.Pos) Token {
	l.advance() // opening backtick
	for {
		switch l.peek(0) {
		case -1:
			t := l.tok(Illegal, start)
			l.diags.Errorf(start, diag.Pos{}, "unterminated raw string")
			return t
		case '`':
			l.advance()
			t := l.tok(String, start)
			t.Value = t.Text[1 : len(t.Text)-1]
			return t
		default:
			l.advance()
		}
	}
}

func (l *lexer) lineComment() {
	start := l.pos()
	for r := l.peek(0); r != -1 && r != '\n'; r = l.peek(0) {
		l.advance()
	}
	l.comments = append(l.comments, Comment{Text: strings.TrimRight(string(l.src[start.Offset:l.off]), "\r"), Pos: start, End: l.pos(), After: l.prevKind, AfterEnd: l.prevEnd})
}

func (l *lexer) blockComment() {
	start := l.pos()
	l.advance()
	l.advance()
	for {
		r := l.peek(0)
		if r == -1 {
			l.diags.Errorf(start, diag.Pos{}, "unterminated comment").Suggestion = `close it with "*/"`
			break
		}
		if r == '*' && l.peek(1) == '/' {
			l.advance()
			l.advance()
			break
		}
		l.advance()
	}
	l.comments = append(l.comments, Comment{Text: string(l.src[start.Offset:l.off]), Pos: start, End: l.pos(), After: l.prevKind, AfterEnd: l.prevEnd})
}

func (l *lexer) docComment() Token {
	start := l.pos()
	for r := l.peek(0); r != -1 && r != '\n'; r = l.peek(0) {
		l.advance()
	}
	t := l.tok(DocComment, start)
	t.after, t.afterEnd = l.prevKind, l.prevEnd
	t.Text = strings.TrimRight(t.Text, "\r")
	v := strings.TrimPrefix(t.Text, "///")
	t.Value = strings.TrimPrefix(v, " ")
	return t
}
