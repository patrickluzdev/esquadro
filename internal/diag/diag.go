// Package diag defines source positions and diagnostics (errors and warnings)
// and renders them compiler-style, with the offending source line and a caret
// marker under the problem.
package diag

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Pos is a position in a source file. Line and Col are 1-based; Col counts
// runes, not bytes. Offset is the 0-based byte offset.
type Pos struct {
	File   string
	Line   int
	Col    int
	Offset int
}

// IsValid reports whether the position points somewhere.
func (p Pos) IsValid() bool { return p.Line > 0 }

func (p Pos) String() string {
	if !p.IsValid() {
		if p.File != "" {
			return p.File
		}
		return "-"
	}
	return p.File + ":" + strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Col)
}

// Severity of a diagnostic.
type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Diagnostic is a single problem found in a source file.
type Diagnostic struct {
	Severity Severity
	Pos      Pos
	// End is the position just after the offending text. When it is invalid
	// or on another line, a single caret is drawn.
	End        Pos
	Message    string
	Suggestion string
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s: %s: %s", d.Pos, d.Severity, d.Message)
}

// List is an ordered collection of diagnostics.
type List []Diagnostic

// Add appends a diagnostic.
func (l *List) Add(d Diagnostic) { *l = append(*l, d) }

// Errorf appends an error diagnostic spanning [pos, end).
func (l *List) Errorf(pos, end Pos, format string, args ...any) *Diagnostic {
	*l = append(*l, Diagnostic{Severity: Error, Pos: pos, End: end, Message: fmt.Sprintf(format, args...)})
	return &(*l)[len(*l)-1]
}

// Warnf appends a warning diagnostic spanning [pos, end).
func (l *List) Warnf(pos, end Pos, format string, args ...any) *Diagnostic {
	*l = append(*l, Diagnostic{Severity: Warning, Pos: pos, End: end, Message: fmt.Sprintf(format, args...)})
	return &(*l)[len(*l)-1]
}

// HasErrors reports whether the list contains at least one error.
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Sort orders diagnostics by file, then position.
func (l List) Sort() {
	sort.SliceStable(l, func(i, j int) bool {
		a, b := l[i].Pos, l[j].Pos
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Offset < b.Offset
	})
}

// Err returns the list as an error when it contains errors, or nil.
func (l List) Err() error {
	if !l.HasErrors() {
		return nil
	}
	errs := make([]error, 0, len(l))
	for _, d := range l {
		if d.Severity == Error {
			errs = append(errs, d)
		}
	}
	return errors.Join(errs...)
}

// SourceFunc returns the contents of a file by name, or nil when unknown.
type SourceFunc func(file string) []byte

const tabWidth = 4

// Render writes every diagnostic in the list, each followed by a blank line,
// and a summary line at the end.
func Render(w io.Writer, l List, src SourceFunc) {
	var nerr, nwarn int
	for _, d := range l {
		var text []byte
		if src != nil {
			text = src(d.Pos.File)
		}
		RenderOne(w, d, text)
		fmt.Fprintln(w)
		if d.Severity == Error {
			nerr++
		} else {
			nwarn++
		}
	}
	switch {
	case nerr > 0 && nwarn > 0:
		fmt.Fprintf(w, "%s, %s\n", plural(nerr, "error", "errors"), plural(nwarn, "warning", "warnings"))
	case nerr > 0:
		fmt.Fprintln(w, plural(nerr, "error", "errors"))
	case nwarn > 0:
		fmt.Fprintln(w, plural(nwarn, "warning", "warnings"))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// RenderOne writes a single diagnostic. src is the full contents of the file
// the diagnostic points to; when nil, only the header line is written.
//
//	shop.esq:12:15: error: unknown entity "Categry"
//	   |
//	12 |   category -> Categry (name)
//	   |               ^^^^^^^
//	   = help: did you mean "Category"?
func RenderOne(w io.Writer, d Diagnostic, src []byte) {
	fmt.Fprintf(w, "%s: %s: %s\n", d.Pos, d.Severity, d.Message)
	line, ok := lineAt(src, d.Pos.Line)
	gutter := strings.Repeat(" ", len(strconv.Itoa(d.Pos.Line)))
	if ok && d.Pos.Col > 0 {
		shown, caretAt := expandTabs(line, d.Pos.Col)
		width := 1
		if d.End.IsValid() && d.End.Line == d.Pos.Line && d.End.Col > d.Pos.Col {
			_, endAt := expandTabs(line, d.End.Col)
			width = endAt - caretAt
		}
		fmt.Fprintf(w, "%s |\n", gutter)
		fmt.Fprintf(w, "%d | %s\n", d.Pos.Line, shown)
		fmt.Fprintf(w, "%s | %s%s\n", gutter, strings.Repeat(" ", caretAt), strings.Repeat("^", width))
	}
	if d.Suggestion != "" {
		fmt.Fprintf(w, "%s = help: %s\n", gutter, d.Suggestion)
	}
}

// lineAt returns the n-th (1-based) line of src without its line terminator.
func lineAt(src []byte, n int) (string, bool) {
	if src == nil || n < 1 {
		return "", false
	}
	for i := 1; i < n; i++ {
		j := strings.IndexByte(string(src), '\n')
		if j < 0 {
			return "", false
		}
		src = src[j+1:]
	}
	if j := strings.IndexByte(string(src), '\n'); j >= 0 {
		src = src[:j]
	}
	return strings.TrimSuffix(string(src), "\r"), true
}

// expandTabs replaces tabs with spaces and returns the display column (0-based)
// of the rune at the 1-based rune column col.
func expandTabs(line string, col int) (string, int) {
	var b strings.Builder
	at, runeIdx, display := -1, 1, 0
	for len(line) > 0 {
		r, size := utf8.DecodeRuneInString(line)
		line = line[size:]
		if runeIdx == col {
			at = display
		}
		if r == '\t' {
			n := tabWidth - display%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			display += n
		} else {
			b.WriteRune(r)
			display++
		}
		runeIdx++
	}
	if at < 0 { // column past end of line (e.g. "unexpected end of line")
		at = display + (col - runeIdx)
		if at < display {
			at = display
		}
	}
	return b.String(), at
}
