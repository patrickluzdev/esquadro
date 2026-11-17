package dsl

import (
	"math"
	"strings"
	"unicode/utf8"
)

// Format returns the canonical source for a file that parsed without errors.
//
// The canonical form uses two-space indentation, one blank line between
// declarations, at most one blank line inside blocks, and aligns the columns
// of consecutive fields, relations and options (a blank line starts a new
// alignment section). All comments are kept. Values keep their original
// spelling, so a backtick string stays a backtick string.
func Format(f *File) []byte {
	p := &printer{comments: f.Comments}
	var out strings.Builder
	prevWasDecl := false
	for _, d := range f.Decls {
		start := d.SourceSpan().Pos
		for _, c := range p.takeComments(start.Offset) {
			if p.lastLine > 0 && (prevWasDecl || c.Pos.Line > p.lastLine+1) {
				out.WriteString("\n")
			}
			out.WriteString(c.Text)
			out.WriteString("\n")
			p.lastLine = c.End.Line
			prevWasDecl = false
		}
		if p.lastLine > 0 && (prevWasDecl || firstLine(d) > p.lastLine+1) {
			out.WriteString("\n")
		}
		switch d := d.(type) {
		case *AppDecl:
			p.app(&out, d)
		case *EnumDecl:
			p.enum(&out, d)
		case *EntityDecl:
			p.entity(&out, "entity", d)
		case *JobDecl:
			p.entity(&out, "job", &d.EntityDecl)
		case *ReportDecl:
			p.entity(&out, "report", &d.EntityDecl)
		case *BlueprintDecl:
			p.blueprint(&out, d)
		case *PackageDecl:
			out.WriteString("package ")
			out.WriteString(d.Name.Name)
			out.WriteString(p.closeTrail(d.Span))
			out.WriteString("\n")
		}
		p.lastLine = d.SourceSpan().End.Line
		prevWasDecl = true
	}
	for _, c := range p.takeComments(math.MaxInt) {
		if p.lastLine > 0 && (prevWasDecl || c.Pos.Line > p.lastLine+1) {
			out.WriteString("\n")
		}
		out.WriteString(c.Text)
		out.WriteString("\n")
		p.lastLine = c.End.Line
		prevWasDecl = false
	}
	return []byte(out.String())
}

// firstLine is the source line where a declaration starts, including its doc
// comment.
func firstLine(d Decl) int {
	var doc []string
	switch d := d.(type) {
	case *AppDecl:
		doc = d.Doc
	case *EnumDecl:
		doc = d.Doc
	case *EntityDecl:
		doc = d.Doc
	case *JobDecl:
		doc = d.Doc
	case *ReportDecl:
		doc = d.Doc
	case *BlueprintDecl:
		doc = d.Doc
	}
	return d.SourceSpan().Pos.Line - len(doc)
}

type printer struct {
	comments []Comment
	ci       int
	lastLine int // last source line already written
}

// takeComments returns the pending comments that start before offset.
func (p *printer) takeComments(offset int) []Comment {
	var out []Comment
	for p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < offset {
		out = append(out, p.comments[p.ci])
		p.ci++
	}
	return out
}

// trailing returns the comment that directly follows a node whose last
// token ends at endOffset, on the same line, if any.
func (p *printer) trailing(endOffset int) string {
	if p.ci < len(p.comments) {
		c := p.comments[p.ci]
		if c.AfterEnd == endOffset && c.After != Newline && c.Pos.Line == c.End.Line {
			p.ci++
			return c.Text
		}
	}
	return ""
}

// headerTrail returns the comment after the "{" that opens a block on the
// header line, as long as it comes before the block's first member.
func (p *printer) headerTrail(headerLine, firstMember int) string {
	if p.ci < len(p.comments) {
		c := p.comments[p.ci]
		if c.After == LBrace && c.Pos.Line == headerLine && c.Pos.Line == c.End.Line && c.Pos.Offset < firstMember {
			p.ci++
			return " " + c.Text
		}
	}
	return ""
}

// ---- block content ----

// line is one output line inside a block: a blank line, free text (comments,
// docs, annotations, nested blocks) or an aligned row of cells.
type line struct {
	blank bool
	text  string   // may contain newlines for nested blocks
	cells []string // aligned row; the last cell is never padded
	kind  rowKind  // rows of different kinds align only their first column
	trail string   // trailing comment
}

type rowKind int

const (
	fieldRow rowKind = iota + 1
	relationRow
	optionRow
	optionDeclRow
	envDeclRow
	keyRow
	goRequireRow
)

// body collects the lines of a block while tracking blank lines and
// comments from the source.
type body struct {
	p     *printer
	lines []line
}

// before adds the comments that precede a member starting at offset and a
// blank line if the source had one before startLine.
func (b *body) before(offset, startLine int) {
	for _, c := range b.p.takeComments(offset) {
		b.gap(c.Pos.Line)
		b.lines = append(b.lines, line{text: c.Text})
		b.p.lastLine = c.End.Line
	}
	b.gap(startLine)
}

func (b *body) gap(startLine int) {
	if len(b.lines) > 0 && !b.lines[len(b.lines)-1].blank && startLine > b.p.lastLine+1 {
		b.lines = append(b.lines, line{blank: true})
	}
}

func (b *body) docs(doc []string) {
	for _, d := range doc {
		b.lines = append(b.lines, line{text: docLine(d)})
	}
}

func docLine(d string) string {
	if d == "" {
		return "///"
	}
	return "/// " + d
}

// end adds the comments left before the closing brace at offset.
func (b *body) end(closeOffset int) {
	for _, c := range b.p.takeComments(closeOffset) {
		b.gap(c.Pos.Line)
		b.lines = append(b.lines, line{text: c.Text})
		b.p.lastLine = c.End.Line
	}
}

// render writes the lines with the given indentation, aligning the cells of
// rows within each section between blank lines.
func (b *body) render(out *strings.Builder, indent string) {
	lines := b.lines
	for len(lines) > 0 && lines[len(lines)-1].blank {
		lines = lines[:len(lines)-1]
	}
	start := 0
	for i := 0; i <= len(lines); i++ {
		if i < len(lines) && !lines[i].blank {
			continue
		}
		renderSection(out, indent, lines[start:i])
		if i < len(lines) {
			out.WriteString("\n")
		}
		start = i + 1
	}
}

func width(s string) int { return utf8.RuneCountInString(s) }

func renderSection(out *strings.Builder, indent string, lines []line) {
	widths := columnWidths(lines)
	rendered := make([]string, len(lines))
	trailWidth := 0
	for i, l := range lines {
		if l.cells == nil {
			rendered[i] = l.text
		} else {
			var sb strings.Builder
			for j, c := range l.cells {
				if j > 0 && c == "" {
					continue
				}
				if j > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(c)
				if j < len(widths[i]) && hasLaterCell(l.cells, j) {
					sb.WriteString(strings.Repeat(" ", widths[i][j]-width(c)))
				}
			}
			rendered[i] = sb.String()
		}
		if l.trail != "" && !strings.Contains(rendered[i], "\n") {
			trailWidth = max(trailWidth, width(rendered[i]))
		}
	}
	for i, l := range lines {
		text := rendered[i]
		if l.trail != "" {
			pad := 1
			if !strings.Contains(text, "\n") {
				pad = trailWidth - width(text) + 1
			}
			text += strings.Repeat(" ", pad) + l.trail
		}
		for ln := range strings.SplitSeq(text, "\n") {
			if ln == "" {
				out.WriteString("\n")
			} else {
				out.WriteString(indent)
				out.WriteString(ln)
				out.WriteString("\n")
			}
		}
	}
}

// columnWidths returns the padded width of each cell of each row. The first
// column (names) aligns across the whole section; later columns align only
// within a run of consecutive rows of the same kind, so a long relation target
// does not push the types of nearby fields to the right. A cell is padded only
// when a later non-empty cell follows it, so lines never end with spaces.
func columnWidths(lines []line) [][]int {
	widths := make([][]int, len(lines))
	first := 0
	for _, l := range lines {
		if len(l.cells) > 1 && hasLaterCell(l.cells, 0) {
			first = max(first, width(l.cells[0]))
		}
	}
	for i := 0; i < len(lines); {
		if lines[i].cells == nil {
			i++
			continue
		}
		kind := lines[i].kind
		var rows []int
		end := i
		for ; end < len(lines) && (lines[end].cells == nil || lines[end].kind == kind); end++ {
			if lines[end].cells != nil {
				rows = append(rows, end)
			}
		}
		run := []int{first}
		for _, r := range rows {
			cells := lines[r].cells
			for j := 1; j < len(cells)-1 && hasLaterCell(cells, j); j++ {
				if j >= len(run) {
					run = append(run, 0)
				}
				run[j] = max(run[j], width(cells[j]))
			}
		}
		for _, r := range rows {
			widths[r] = run
		}
		i = end
	}
	return widths
}

func hasLaterCell(cells []string, j int) bool {
	for _, c := range cells[j+1:] {
		if c != "" {
			return true
		}
	}
	return false
}

// ---- declarations ----

func (p *printer) app(out *strings.Builder, d *AppDecl) {
	p.docs(out, d.Doc)
	header := "app " + d.Name.Name + " {"
	first := d.End.Offset
	if len(d.Items) > 0 {
		first = d.Items[0].SourceSpan().Pos.Offset
	}
	header += p.headerTrail(d.Pos.Line, first)
	out.WriteString(header)
	out.WriteString("\n")
	p.lastLine = d.Pos.Line
	b := &body{p: p}
	for _, it := range d.Items {
		sp := it.SourceSpan()
		b.before(sp.Pos.Offset, sp.Pos.Line)
		var l line
		switch it := it.(type) {
		case *ModuleItem:
			l.text = "module " + valueString(it.Path)
		case *BlueprintRef:
			l.text = p.blueprintRef(it)
		case *Annotation:
			l.text = annotationString(it)
		case *EnvBlock:
			l.text = p.envBlock(it)
		}
		l.trail = p.trailing(sp.End.Offset)
		b.lines = append(b.lines, l)
		p.lastLine = sp.End.Line
	}
	b.end(d.End.Offset - 1)
	b.render(out, "  ")
	out.WriteString("}")
	out.WriteString(p.closeTrail(d.Span))
	out.WriteString("\n")
}

// envBlock writes an app's env block: declarations, or an environment's
// values.
func (p *printer) envBlock(e *EnvBlock) string {
	var sb strings.Builder
	sb.WriteString("env")
	if e.Environment != nil {
		sb.WriteString(" ")
		sb.WriteString(e.Environment.Name)
	}
	first := e.End.Offset - 1
	switch {
	case len(e.Decls) > 0:
		first = e.Decls[0].Pos.Offset
	case len(e.Values) > 0:
		first = e.Values[0].Pos.Offset
	}
	sb.WriteString(" {")
	sb.WriteString(p.headerTrail(e.Pos.Line, first))
	sb.WriteString("\n")
	p.lastLine = e.Pos.Line
	b := &body{p: p}
	for _, d := range e.Decls {
		b.before(d.Pos.Offset, d.Pos.Line-len(d.Doc))
		b.docs(d.Doc)
		b.lines = append(b.lines, line{
			cells: []string{d.Name.Name, d.Type.Name, modifiersString(d.Modifiers, nil)},
			kind:  keyRow,
			trail: p.trailing(d.End.Offset),
		})
		p.lastLine = d.End.Line
	}
	for _, o := range e.Values {
		b.before(o.Pos.Offset, o.Pos.Line)
		b.lines = append(b.lines, line{
			cells: []string{o.Key.Name, valueString(o.Value)},
			kind:  optionRow,
			trail: p.trailing(o.End.Offset),
		})
		p.lastLine = o.End.Line
	}
	b.end(e.End.Offset - 1)
	var inner strings.Builder
	b.render(&inner, "  ")
	sb.WriteString(inner.String())
	sb.WriteString("}")
	return sb.String()
}

func (p *printer) blueprintRef(r *BlueprintRef) string {
	head := "blueprint " + valueString(r.Ref)
	innerComments := p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < r.End.Offset
	if len(r.Options) == 0 && !innerComments {
		// An empty "{ }" is dropped.
		return head
	}
	first := r.End.Offset - 1
	if len(r.Options) > 0 {
		first = r.Options[0].Pos.Offset
	}
	var sb strings.Builder
	sb.WriteString(head)
	sb.WriteString(" {")
	sb.WriteString(p.headerTrail(r.Pos.Line, first))
	sb.WriteString("\n")
	p.lastLine = r.Pos.Line
	b := &body{p: p}
	for _, o := range r.Options {
		b.before(o.Pos.Offset, o.Pos.Line)
		b.lines = append(b.lines, line{
			cells: []string{o.Key.Name, valueString(o.Value)},
			kind:  optionRow,
			trail: p.trailing(o.End.Offset),
		})
		p.lastLine = o.End.Line
	}
	b.end(r.End.Offset - 1)
	var inner strings.Builder
	b.render(&inner, "  ")
	sb.WriteString(inner.String())
	sb.WriteString("}")
	return sb.String()
}

func (p *printer) enum(out *strings.Builder, d *EnumDecl) {
	p.docs(out, d.Doc)
	hasDocs := false
	for _, v := range d.Values {
		hasDocs = hasDocs || len(v.Doc) > 0
	}
	innerComments := p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < d.End.Offset
	if d.Pos.Line == d.End.Line && !hasDocs && !innerComments {
		names := make([]string, len(d.Values))
		for i, v := range d.Values {
			names[i] = v.Name.Name
		}
		s := "enum " + d.Name.Name + " { " + strings.Join(names, " ") + " }"
		if len(names) == 0 {
			s = "enum " + d.Name.Name + " {}"
		}
		out.WriteString(s)
		out.WriteString(p.closeTrail(d.Span))
		out.WriteString("\n")
		return
	}
	first := d.End.Offset
	if len(d.Values) > 0 {
		first = d.Values[0].Name.Pos.Offset
	}
	header := "enum " + d.Name.Name + " {" + p.headerTrail(d.Pos.Line, first)
	out.WriteString(header)
	out.WriteString("\n")
	p.lastLine = d.Pos.Line
	b := &body{p: p}
	for _, v := range d.Values {
		b.before(v.Name.Pos.Offset, v.Name.Pos.Line-len(v.Doc))
		b.docs(v.Doc)
		b.lines = append(b.lines, line{text: v.Name.Name, trail: p.trailing(v.Name.End.Offset)})
		p.lastLine = v.Name.Pos.Line
	}
	b.end(d.End.Offset - 1)
	b.render(out, "  ")
	out.WriteString("}")
	out.WriteString(p.closeTrail(d.Span))
	out.WriteString("\n")
}

// entity writes an entity, a report, or a job (keyword "job"), which is
// written without a block when it has nothing in it.
func (p *printer) entity(out *strings.Builder, keyword string, d *EntityDecl) {
	p.docs(out, d.Doc)
	header := keyword + " " + d.Name.Name
	if len(d.Options) > 0 {
		opts := make([]string, len(d.Options))
		for i, o := range d.Options {
			opts[i] = o.Key.Name + " " + valueString(o.Value)
		}
		header += " (" + strings.Join(opts, ", ") + ")"
	}
	innerComments := p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < d.End.Offset-1
	if len(d.Members) == 0 && !innerComments {
		out.WriteString(header)
		if keyword != "job" {
			out.WriteString(" {}")
		}
		out.WriteString(p.closeTrail(d.Span))
		out.WriteString("\n")
		return
	}
	first := d.End.Offset
	if len(d.Members) > 0 {
		first = memberStart(d.Members[0])
	}
	header += " {" + p.headerTrail(d.Pos.Line, first)
	out.WriteString(header)
	out.WriteString("\n")
	p.lastLine = d.Pos.Line
	b := &body{p: p}
	for _, m := range d.Members {
		var sp Span
		var doc []string
		var l line
		switch m := m.(type) {
		case *Field:
			sp, doc = m.Span, m.Doc
			l.cells = []string{m.Name.Name, m.Type.Name, modifiersString(m.Modifiers, m.Annotations)}
			l.kind = fieldRow
		case *Relation:
			sp, doc = m.Span, m.Doc
			target := m.Kind.String() + " " + m.Target.Name
			if m.TargetField != nil {
				target += "." + m.TargetField.Name
			}
			if m.Display != nil {
				target += " (" + m.Display.Name + ")"
			}
			l.cells = []string{m.Name.Name, target, modifiersString(m.Modifiers, m.Annotations)}
			l.kind = relationRow
		case *Annotation:
			sp = m.Span
			l.text = annotationString(m)
		}
		b.before(sp.Pos.Offset, sp.Pos.Line-len(doc))
		b.docs(doc)
		l.trail = p.trailing(sp.End.Offset)
		b.lines = append(b.lines, l)
		p.lastLine = sp.End.Line
	}
	b.end(d.End.Offset - 1)
	b.render(out, "  ")
	out.WriteString("}")
	out.WriteString(p.closeTrail(d.Span))
	out.WriteString("\n")
}

func memberStart(m Member) int {
	switch m := m.(type) {
	case *Field:
		return m.Pos.Offset
	case *Relation:
		return m.Pos.Offset
	case *Annotation:
		return m.Pos.Offset
	}
	return 0
}

func (p *printer) docs(out *strings.Builder, doc []string) {
	for _, d := range doc {
		out.WriteString(docLine(d))
		out.WriteString("\n")
	}
}

// closeTrail returns " // comment" for a comment after a closing brace.
func (p *printer) closeTrail(s Span) string {
	if t := p.trailing(s.End.Offset); t != "" {
		return " " + t
	}
	return ""
}

// ---- values ----

func valueString(v Value) string {
	if v.Kind == ListValue {
		parts := make([]string, len(v.List))
		for i, e := range v.List {
			parts[i] = valueString(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	if v.Raw != "" {
		return v.Raw
	}
	return v.Text
}

func modifiersString(mods []*Modifier, anns []*Annotation) string {
	parts := make([]string, 0, len(mods)+len(anns))
	for _, m := range mods {
		s := m.Name.Name
		if m.HasParens {
			args := make([]string, len(m.Args))
			for i, a := range m.Args {
				args[i] = valueString(a)
			}
			s += "(" + strings.Join(args, ", ") + ")"
		}
		parts = append(parts, s)
	}
	for _, a := range anns {
		parts = append(parts, annotationString(a))
	}
	return strings.Join(parts, " ")
}

func annotationString(a *Annotation) string {
	if len(a.Args) == 0 {
		return "@" + a.Name.Name
	}
	args := make([]string, len(a.Args))
	for i, arg := range a.Args {
		args[i] = arg.Key.Name
		if arg.Value != nil {
			args[i] += " " + valueString(*arg.Value)
		}
	}
	return "@" + a.Name.Name + "(" + strings.Join(args, ", ") + ")"
}

// ---- blueprint manifests ----

func (p *printer) blueprint(out *strings.Builder, d *BlueprintDecl) {
	p.docs(out, d.Doc)
	first := d.End.Offset
	if len(d.Items) > 0 {
		first = d.Items[0].SourceSpan().Pos.Offset
	}
	out.WriteString("blueprint ")
	out.WriteString(d.Name.Name)
	out.WriteString(" {")
	out.WriteString(p.headerTrail(d.Pos.Line, first))
	out.WriteString("\n")
	p.lastLine = d.Pos.Line
	b := &body{p: p}
	for _, it := range d.Items {
		sp := it.SourceSpan()
		// Comments and blank lines before the item come first: building a
		// nested block below consumes the comments inside it.
		doc := manifestItemDoc(it)
		b.before(sp.Pos.Offset, sp.Pos.Line-len(doc))
		b.docs(doc)
		var l line
		switch it := it.(type) {
		case *VersionItem:
			l.text = "version " + valueString(it.Version)
		case *DependsItem:
			names := make([]string, len(it.Names))
			for i, n := range it.Names {
				names[i] = n.Name
			}
			l.text = it.Keyword.Name + " " + strings.Join(names, ", ")
		case *OptionDecl:
			l.cells = []string{"option " + it.Name.Name, it.Type.Name, modifiersString(it.Modifiers, nil)}
			l.kind = optionDeclRow
		case *EnvDecl:
			l.cells = []string{"env " + it.Name.Name, it.Type.Name, modifiersString(it.Modifiers, nil)}
			l.kind = envDeclRow
		case *AnnotationDecl:
			l.text = p.annotationDecl(it)
		case *SlotItem:
			l.text = "slot " + valueString(it.Name)
		case *GoRequireItem:
			l.cells = []string{"go_require " + valueString(it.Module), valueString(it.Version)}
			l.kind = goRequireRow
		case *HookItem:
			l.text = "hook " + it.Event.Name + " " + valueString(it.Path)
			if it.Writes != nil {
				l.text += " writes " + valueString(*it.Writes)
			}
			if it.Edits != nil {
				l.text += " edits " + valueString(*it.Edits)
			}
		}
		l.trail = p.trailing(sp.End.Offset)
		b.lines = append(b.lines, l)
		p.lastLine = sp.End.Line
	}
	b.end(d.End.Offset - 1)
	b.render(out, "  ")
	out.WriteString("}")
	out.WriteString(p.closeTrail(d.Span))
	out.WriteString("\n")
}

func manifestItemDoc(it ManifestItem) []string {
	switch it := it.(type) {
	case *OptionDecl:
		return it.Doc
	case *EnvDecl:
		return it.Doc
	case *AnnotationDecl:
		return it.Doc
	case *SlotItem:
		return it.Doc
	}
	return nil
}

func (p *printer) annotationDecl(a *AnnotationDecl) string {
	var sb strings.Builder
	sb.WriteString("annotation ")
	sb.WriteString(a.Target.Name)
	if len(a.Keys) == 0 && !(p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < a.End.Offset-1) {
		sb.WriteString(" {}")
		return sb.String()
	}
	first := a.End.Offset
	if len(a.Keys) > 0 {
		first = a.Keys[0].Pos.Offset
	}
	sb.WriteString(" {")
	sb.WriteString(p.headerTrail(a.Pos.Line, first))
	sb.WriteString("\n")
	p.lastLine = a.Pos.Line
	b := &body{p: p}
	for _, k := range a.Keys {
		b.before(k.Pos.Offset, k.Pos.Line-len(k.Doc))
		b.docs(k.Doc)
		b.lines = append(b.lines, line{
			cells: []string{k.Name.Name, k.Type.Name, modifiersString(k.Modifiers, nil)},
			kind:  keyRow,
			trail: p.trailing(k.End.Offset),
		})
		p.lastLine = k.End.Line
	}
	b.end(a.End.Offset - 1)
	var inner strings.Builder
	b.render(&inner, "  ")
	sb.WriteString(inner.String())
	sb.WriteString("}")
	return sb.String()
}
