package lsp

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Position is a place in a document as the protocol counts it: line and
// character from 0, the character in UTF-16 code units.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a span of a document, End excluded.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// text is a document's content, split in lines for converting positions.
type text struct {
	src   string
	lines []string // without their "\n"
}

func newText(src string) text {
	return text{src: src, lines: strings.Split(src, "\n")}
}

// line is line i (from 0), without "\r", or "" past the end.
func (t text) line(i int) string {
	if i < 0 || i >= len(t.lines) {
		return ""
	}
	return strings.TrimSuffix(t.lines[i], "\r")
}

// fromRunes converts esquadro's position (line and column from 1, the column
// in runes) to the protocol's.
func (t text) fromRunes(line, col int) Position {
	l := t.line(line - 1)
	units, runes := 0, 0
	for _, r := range l {
		if runes >= col-1 {
			break
		}
		units += utf16.RuneLen(r)
		runes++
	}
	// A column past the end of the line (an error at the end of a line)
	// stays past it.
	units += max(0, col-1-runes)
	return Position{Line: max(line-1, 0), Character: units}
}

// offset is the byte offset of a protocol position, clamped to the text.
func (t text) offset(p Position) int {
	off := 0
	for i := 0; i < p.Line && i < len(t.lines); i++ {
		off += len(t.lines[i]) + 1
	}
	if p.Line >= len(t.lines) {
		return len(t.src)
	}
	l := t.lines[p.Line]
	units := 0
	for i, r := range l {
		if units >= p.Character {
			return off + i
		}
		units += utf16.RuneLen(r)
	}
	return off + len(l)
}

// end is the protocol position of the end of the text.
func (t text) end() Position {
	last := len(t.lines) - 1
	return Position{Line: last, Character: utf16Len(t.lines[last])}
}

// wordEnd is the column (in runes, from 1) after the word or symbol that
// starts at col on line, for errors esquadro reports at a single position.
func (t text) wordEnd(line, col int) int {
	runes := []rune(t.line(line - 1))
	i := col - 1
	if i < 0 || i >= len(runes) {
		return col + 1
	}
	if !isWordRune(runes[i]) {
		return col + 1
	}
	for i < len(runes) && isWordRune(runes[i]) {
		i++
	}
	return i + 1
}

func isWordRune(r rune) bool {
	return r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= utf8.RuneSelf
}

// utf16Len is the length of s in UTF-16 code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// apply returns src with the span r replaced by s; a nil r replaces it all.
func apply(src string, r *Range, s string) string {
	if r == nil {
		return s
	}
	t := newText(src)
	start, end := t.offset(r.Start), t.offset(r.End)
	if end < start {
		start, end = end, start
	}
	return src[:start] + s + src[end:]
}
