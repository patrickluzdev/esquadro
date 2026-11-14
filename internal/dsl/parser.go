package dsl

import (
	"slices"
	"sort"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

var (
	topKeywords      = []string{"app", "enum", "entity", "blueprint"}
	appSettings      = []string{"module", "blueprint"}
	manifestSettings = []string{"version", "requires", "after", "option", "annotation", "slot", "go_require", "hook"}
)

// Parse parses a .esq file. It always returns a File, possibly partial, plus
// every syntax error found: after an error the parser skips to the next line
// or declaration and keeps going, so one run reports all problems.
func Parse(filename string, src []byte) (*File, diag.List) {
	var diags diag.List
	toks, comments := lex(filename, src, &diags)
	p := &parser{toks: toks, diags: &diags}
	p.collectDocs()

	f := &File{Name: filename, Comments: comments}
	for {
		p.skipNewlines()
		t := p.tok()
		if t.Kind == EOF {
			break
		}
		var d Decl
		if t.Kind == Name {
			switch t.Text {
			case "app":
				d = p.parseApp()
			case "enum":
				d = p.parseEnum()
			case "entity":
				d = p.parseEntity()
			case "blueprint":
				d = p.parseBlueprint()
			}
		}
		if d == nil {
			// A failed app/enum/entity has already reported and resynced.
			if t.Kind != Name || !isTopKeyword(t.Text) {
				e := p.errorf(t, "expected app, enum, entity or blueprint, found %s", t.describe())
				if t.Kind == Name {
					e.Suggestion = didYouMean(t.Text, topKeywords)
				}
				p.syncTop()
			}
			continue
		}
		f.Decls = append(f.Decls, d)
	}
	p.dropDoc()
	if len(p.orphans) > 0 {
		f.Comments = append(f.Comments, p.orphans...)
		sort.SliceStable(f.Comments, func(i, j int) bool { return f.Comments[i].Pos.Offset < f.Comments[j].Pos.Offset })
	}
	diags.Sort()
	return f, firstPerLine(diags)
}

type parser struct {
	toks  []Token
	i     int
	diags *diag.List
	doc   []Token
	// orphans are doc comments that no node claimed (e.g. before a "}");
	// they are kept as regular comments so the formatter never drops them.
	orphans []Comment
	lastEnd diag.Pos
}

func (p *parser) tok() Token { return p.toks[p.i] }

// advance consumes the current token and returns it. Doc comments are never
// returned as tokens; they are buffered and claimed with takeDoc.
func (p *parser) advance() Token {
	t := p.toks[p.i]
	if t.Kind != EOF {
		p.i++
		p.lastEnd = t.End
	}
	p.collectDocs()
	return t
}

func (p *parser) collectDocs() {
	for p.toks[p.i].Kind == DocComment {
		p.doc = append(p.doc, p.toks[p.i])
		p.i++
		// The newline ending the doc comment is not significant.
		if p.toks[p.i].Kind == Newline {
			p.i++
		}
	}
}

func (p *parser) takeDoc() []string {
	var d []string
	for _, t := range p.doc {
		d = append(d, t.Value)
	}
	p.doc = nil
	return d
}

// dropDoc turns pending doc comments into regular comments.
func (p *parser) dropDoc() {
	for _, t := range p.doc {
		p.orphans = append(p.orphans, Comment{Text: t.Text, Pos: t.Pos, End: t.End, After: t.after, AfterEnd: t.afterEnd})
	}
	p.doc = nil
}

func (p *parser) skipNewlines() {
	for p.tok().Kind == Newline {
		p.advance()
	}
}

// errorf reports an error at token t. Illegal tokens were already reported by
// the lexer, so errors on them are dropped to avoid duplicates.
func (p *parser) errorf(t Token, format string, args ...any) *diag.Diagnostic {
	if t.Kind == Illegal {
		return &diag.Diagnostic{}
	}
	end := t.End
	if t.Kind == Newline || t.Kind == EOF {
		end = diag.Pos{}
	}
	return p.diags.Errorf(t.Pos, end, format, args...)
}

func (p *parser) expect(kind Kind, context string) (Token, bool) {
	t := p.tok()
	if t.Kind != kind {
		p.errorf(t, "expected %s %s, found %s", kind, context, t.describe())
		return t, false
	}
	return p.advance(), true
}

func (p *parser) expectIdent(what string) (Ident, bool) {
	t := p.tok()
	if t.Kind != Name {
		p.errorf(t, "expected %s, found %s", what, t.describe())
		return Ident{}, false
	}
	p.advance()
	return identOf(t), true
}

func identOf(t Token) Ident {
	return Ident{Name: t.Text, Pos: t.Pos, End: t.End}
}

func isTopKeyword(s string) bool { return slices.Contains(topKeywords, s) }

func didYouMean(name string, cands []string) string {
	if s := diag.Suggest(name, cands); s != "" {
		return `did you mean "` + s + `"?`
	}
	return ""
}

// syncTop skips tokens until the start of the next top-level declaration.
func (p *parser) syncTop() {
	depth := 0
	for {
		t := p.tok()
		switch t.Kind {
		case EOF:
			return
		case LBrace:
			depth++
		case RBrace:
			if depth > 0 {
				depth--
			}
		case Newline:
			p.advance()
			if n := p.tok(); depth == 0 && n.Kind == Name && isTopKeyword(n.Text) {
				return
			}
			continue
		}
		p.advance()
	}
}

// syncLine skips to the end of the current line, without consuming a closing
// "}" so the enclosing block can still end. Nested braces are skipped whole.
func (p *parser) syncLine() {
	depth := 0
	for {
		switch p.tok().Kind {
		case EOF:
			return
		case Newline:
			if depth == 0 {
				return
			}
		case LBrace:
			depth++
		case RBrace:
			if depth == 0 {
				return
			}
			depth--
		}
		p.advance()
	}
}

// atDeclStart reports whether the next tokens look like the start of a
// top-level declaration: a keyword, a name, then "{" or "(". A field named
// like a keyword ("entity string") does not match.
func (p *parser) atDeclStart() bool {
	t := p.tok()
	// "blueprint x {" is also a valid line inside an app block.
	if t.Kind != Name || !isTopKeyword(t.Text) || t.Text == "blueprint" || p.i+2 >= len(p.toks) {
		return false
	}
	name, after := p.toks[p.i+1], p.toks[p.i+2]
	return name.Kind == Name && (after.Kind == LBrace || after.Kind == LParen)
}

// firstPerLine keeps only the first diagnostic on each line. Later errors on
// the same line are almost always cascades of the first one. l must be sorted.
func firstPerLine(l diag.List) diag.List {
	var out diag.List
	for _, d := range l {
		if n := len(out); n > 0 && d.Pos.File == out[n-1].Pos.File && d.Pos.Line == out[n-1].Pos.Line {
			continue
		}
		out = append(out, d)
	}
	return out
}

// parseBlock parses "{ member* }" after the opening brace was consumed. Each
// member must end at a newline or the closing brace. It returns the end
// position of the block.
func (p *parser) parseBlock(open Token, what string, member func() bool) diag.Pos {
	for {
		p.skipNewlines()
		t := p.tok()
		switch t.Kind {
		case RBrace:
			p.dropDoc()
			p.advance()
			return t.End
		case EOF:
			p.errorf(t, `expected "}" to close %s opened at line %d`, what, open.Pos.Line)
			return t.Pos
		}
		if p.atDeclStart() {
			// "entity Product {" inside a block almost always means the block
			// was not closed. Stop here so the declaration parses normally.
			p.errorf(t, `expected "}" to close %s opened at line %d before %s %s`,
				what, open.Pos.Line, t.Text, p.toks[p.i+1].Text)
			return p.lastEnd
		}
		if !member() {
			p.syncLine()
			continue
		}
		switch t := p.tok(); t.Kind {
		case Newline, RBrace, EOF:
		default:
			p.errorf(t, "unexpected %s, expected end of line", t.describe())
			p.syncLine()
		}
	}
}

func (p *parser) parseApp() Decl {
	doc := p.takeDoc()
	start := p.advance() // "app"
	name, ok := p.expectIdent("app name")
	if !ok {
		p.syncTop()
		return nil
	}
	open, ok := p.expect(LBrace, "after app name")
	if !ok {
		p.syncTop()
		return nil
	}
	app := &AppDecl{Doc: doc, Name: name}
	end := p.parseBlock(open, "app "+name.Name, func() bool { return p.parseAppItem(app) })
	app.Span = Span{Pos: start.Pos, End: end}
	return app
}

func (p *parser) parseAppItem(app *AppDecl) bool {
	p.dropDoc()
	t := p.tok()
	if t.Kind == At {
		a := p.parseAnnotation()
		if a == nil {
			return false
		}
		app.Annotations = append(app.Annotations, a)
		app.Items = append(app.Items, a)
		return true
	}
	if t.Kind != Name {
		p.errorf(t, "expected an app setting (module or blueprint), found %s", t.describe())
		return false
	}
	switch t.Text {
	case "module":
		p.advance()
		s := p.tok()
		if s.Kind != String {
			p.errorf(s, "expected module path as a string, found %s", s.describe()).
				Suggestion = `e.g. module "github.com/you/shop"`
			return false
		}
		p.advance()
		v := tokenValue(s)
		if app.Module != nil {
			p.errorf(t, "module is already set at line %d", app.Module.Pos.Line)
			return true
		}
		app.Module = &v
		app.Items = append(app.Items, &ModuleItem{Path: v, Pos: t.Pos, End: s.End})
	case "blueprint":
		p.advance()
		r := p.tok()
		if r.Kind != Name && r.Kind != String {
			p.errorf(r, "expected blueprint name or path, found %s", r.describe())
			return false
		}
		p.advance()
		ref := &BlueprintRef{Ref: tokenValue(r), Pos: t.Pos, End: r.End}
		if p.tok().Kind == LBrace {
			open := p.advance()
			ref.End = p.parseBlock(open, "blueprint options", func() bool {
				o := p.parseOption()
				if o != nil {
					ref.Options = append(ref.Options, o)
				}
				return o != nil
			})
		}
		app.Blueprints = append(app.Blueprints, ref)
		app.Items = append(app.Items, ref)
	default:
		p.errorf(t, "unknown app setting %q", t.Text).Suggestion = didYouMean(t.Text, appSettings)
		return false
	}
	return true
}

func tokenValue(t Token) Value {
	v := Value{Kind: IdentValue, Text: t.Text, Raw: t.Text, Pos: t.Pos, End: t.End}
	switch {
	case t.Kind == String:
		v.Kind, v.Text = StringValue, t.Value
	case t.Kind == Number:
		v.Kind = NumberValue
	case t.Text == "true" || t.Text == "false":
		v.Kind = BoolValue
	}
	return v
}

// parseOption parses "key value".
func (p *parser) parseOption() *Option {
	key, ok := p.expectIdent("option name")
	if !ok {
		return nil
	}
	v, ok := p.parseValue("after option " + key.Name)
	if !ok {
		return nil
	}
	return &Option{Key: key, Value: v, Pos: key.Pos, End: v.End}
}

func (p *parser) parseValue(context string) (Value, bool) {
	t := p.tok()
	switch t.Kind {
	case String, Number, Name:
		p.advance()
		return tokenValue(t), true
	case LBrack:
		p.advance()
		list := Value{Kind: ListValue, Pos: t.Pos}
		for {
			p.skipNewlines()
			if c := p.tok(); c.Kind == RBrack {
				p.advance()
				list.End = c.End
				return list, true
			}
			v, ok := p.parseValue("in list")
			if !ok {
				return list, false
			}
			list.List = append(list.List, v)
			p.skipNewlines()
			switch c := p.tok(); c.Kind {
			case Comma:
				p.advance()
			case RBrack:
			default:
				p.errorf(c, `expected "," or "]" in list, found %s`, c.describe())
				return list, false
			}
		}
	}
	p.errorf(t, "expected a value %s (string, number, true/false, name or [list]), found %s", context, t.describe())
	return Value{}, false
}

func (p *parser) parseEnum() Decl {
	doc := p.takeDoc()
	start := p.advance() // "enum"
	name, ok := p.expectIdent("enum name")
	if !ok {
		p.syncTop()
		return nil
	}
	open, ok := p.expect(LBrace, "after enum name")
	if !ok {
		p.syncTop()
		return nil
	}
	e := &EnumDecl{Doc: doc, Name: name}
	for {
		p.skipNewlines()
		t := p.tok()
		switch t.Kind {
		case RBrace:
			p.dropDoc()
			p.advance()
			e.Span = Span{Pos: start.Pos, End: t.End}
			return e
		case EOF:
			p.errorf(t, `expected "}" to close enum %s opened at line %d`, name.Name, open.Pos.Line)
			e.Span = Span{Pos: start.Pos, End: t.Pos}
			return e
		case Comma:
			p.advance()
		case Name:
			e.Values = append(e.Values, &EnumValue{Doc: p.takeDoc(), Name: identOf(t)})
			p.advance()
		default:
			p.errorf(t, "expected enum value, found %s", t.describe())
			p.syncLine()
		}
	}
}

func (p *parser) parseEntity() Decl {
	doc := p.takeDoc()
	start := p.advance() // "entity"
	name, ok := p.expectIdent("entity name")
	if !ok {
		p.syncTop()
		return nil
	}
	ent := &EntityDecl{Doc: doc, Name: name}
	if p.tok().Kind == LParen {
		if !p.parseEntityOptions(ent) {
			p.syncTop()
			return nil
		}
	}
	open, ok := p.expect(LBrace, "after entity name")
	if !ok {
		p.syncTop()
		return nil
	}
	end := p.parseBlock(open, "entity "+name.Name, func() bool { return p.parseMember(ent) })
	ent.Span = Span{Pos: start.Pos, End: end}
	return ent
}

func (p *parser) parseEntityOptions(ent *EntityDecl) bool {
	p.advance() // "("
	for {
		p.skipNewlines()
		if p.tok().Kind == RParen {
			p.advance()
			return true
		}
		o := p.parseOption()
		if o == nil {
			return false
		}
		ent.Options = append(ent.Options, o)
		p.skipNewlines()
		switch t := p.tok(); t.Kind {
		case Comma:
			p.advance()
		case RParen:
		default:
			p.errorf(t, `expected "," or ")" in entity options, found %s`, t.describe())
			return false
		}
	}
}

func (p *parser) parseMember(ent *EntityDecl) bool {
	t := p.tok()
	if t.Kind == At {
		p.dropDoc()
		a := p.parseAnnotation()
		if a == nil {
			return false
		}
		ent.Annotations = append(ent.Annotations, a)
		ent.Members = append(ent.Members, a)
		return true
	}
	if t.Kind != Name {
		p.errorf(t, "expected a field, relation or @annotation, found %s", t.describe())
		return false
	}
	doc := p.takeDoc()
	name := identOf(p.advance())

	switch k := p.tok(); k.Kind {
	case Arrow, BiArrow:
		p.advance()
		r := &Relation{Doc: doc, Name: name, Kind: ManyToOne}
		if k.Kind == BiArrow {
			r.Kind = ManyToMany
		}
		target, ok := p.expectIdent("target entity after " + k.Kind.String())
		if !ok {
			return false
		}
		r.Target = target
		if p.tok().Kind == LParen {
			p.advance()
			disp, ok := p.expectIdent("display field of " + target.Name)
			if !ok {
				return false
			}
			r.Display = &disp
			if _, ok := p.expect(RParen, "after display field"); !ok {
				return false
			}
		}
		if !p.parseModifiers(&r.Modifiers, &r.Annotations) {
			return false
		}
		r.Span = Span{Pos: name.Pos, End: p.lastEnd}
		ent.Relations = append(ent.Relations, r)
		ent.Members = append(ent.Members, r)
	case Name:
		f := &Field{Doc: doc, Name: name, Type: identOf(p.advance())}
		if !p.parseModifiers(&f.Modifiers, &f.Annotations) {
			return false
		}
		f.Span = Span{Pos: name.Pos, End: p.lastEnd}
		ent.Fields = append(ent.Fields, f)
		ent.Members = append(ent.Members, f)
	default:
		p.errorf(k, "expected a type or a relation (-> or <->) after field %q, found %s", name.Name, k.describe())
		return false
	}
	return true
}

// parseModifiers parses modifiers and annotations, in any order, until the end
// of the line.
func (p *parser) parseModifiers(mods *[]*Modifier, anns *[]*Annotation) bool {
	for {
		t := p.tok()
		switch t.Kind {
		case At:
			a := p.parseAnnotation()
			if a == nil {
				return false
			}
			*anns = append(*anns, a)
		case Name:
			p.advance()
			m := &Modifier{Name: identOf(t), Pos: t.Pos, End: t.End}
			if p.tok().Kind == LParen {
				m.HasParens = true
				p.advance()
				for p.tok().Kind != RParen {
					v, ok := p.parseValue("in " + t.Text + "(...)")
					if !ok {
						return false
					}
					m.Args = append(m.Args, v)
					switch c := p.tok(); c.Kind {
					case Comma:
						p.advance()
					case RParen:
					default:
						p.errorf(c, `expected "," or ")" in %s(...), found %s`, t.Text, c.describe())
						return false
					}
				}
				m.End = p.advance().End
			}
			*mods = append(*mods, m)
		default:
			return true
		}
	}
}

func (p *parser) parseAnnotation() *Annotation {
	at := p.advance() // "@"
	name, ok := p.expectIdent(`annotation name after "@"`)
	if !ok {
		return nil
	}
	a := &Annotation{Name: name, Pos: at.Pos, End: name.End}
	if p.tok().Kind != LParen {
		return a
	}
	p.advance()
	for {
		p.skipNewlines()
		if c := p.tok(); c.Kind == RParen {
			p.advance()
			a.End = c.End
			return a
		}
		key, ok := p.expectIdent("annotation argument name")
		if !ok {
			return nil
		}
		arg := AnnotationArg{Key: key, Span: key.Span}
		if k := p.tok().Kind; k != Comma && k != RParen && k != Newline {
			v, ok := p.parseValue("for " + key.Name)
			if !ok {
				return nil
			}
			arg.Value = &v
			arg.End = v.End
		}
		a.Args = append(a.Args, arg)
		p.skipNewlines()
		switch c := p.tok(); c.Kind {
		case Comma:
			p.advance()
		case RParen:
		default:
			p.errorf(c, `expected "," or ")" in @%s(...), found %s`, name.Name, c.describe())
			return nil
		}
	}
}

// ---- blueprint manifests ----

func (p *parser) parseBlueprint() Decl {
	doc := p.takeDoc()
	start := p.advance() // "blueprint"
	name, ok := p.expectIdent("blueprint name")
	if !ok {
		p.syncTop()
		return nil
	}
	open, ok := p.expect(LBrace, "after blueprint name")
	if !ok {
		p.syncTop()
		return nil
	}
	bp := &BlueprintDecl{Doc: doc, Name: name}
	end := p.parseBlock(open, "blueprint "+name.Name, func() bool {
		it := p.parseManifestItem()
		if it != nil {
			bp.Items = append(bp.Items, it)
		}
		return it != nil
	})
	bp.Span = Span{Pos: start.Pos, End: end}
	return bp
}

func (p *parser) expectString(what string) (Value, bool) {
	t := p.tok()
	if t.Kind != String {
		p.errorf(t, "expected %s as a string, found %s", what, t.describe())
		return Value{}, false
	}
	p.advance()
	return tokenValue(t), true
}

func (p *parser) parseManifestItem() ManifestItem {
	doc := p.takeDoc()
	t := p.tok()
	if t.Kind != Name {
		p.errorf(t, "expected a manifest setting (%s), found %s", strings.Join(manifestSettings, ", "), t.describe())
		return nil
	}
	p.advance()
	switch t.Text {
	case "version":
		v, ok := p.expectString("version")
		if !ok {
			return nil
		}
		return &VersionItem{Version: v, Pos: t.Pos, End: v.End}
	case "requires", "after":
		it := &DependsItem{Keyword: identOf(t)}
		for {
			n, ok := p.expectIdent("blueprint name after " + t.Text)
			if !ok {
				return nil
			}
			it.Names = append(it.Names, n)
			if p.tok().Kind != Comma {
				break
			}
			p.advance()
		}
		it.Span = Span{Pos: t.Pos, End: p.lastEnd}
		return it
	case "option":
		name, ok := p.expectIdent("option name")
		if !ok {
			return nil
		}
		typ, ok := p.expectIdent("option type")
		if !ok {
			return nil
		}
		o := &OptionDecl{Doc: doc, Name: name, Type: typ}
		if !p.parsePlainModifiers(&o.Modifiers) {
			return nil
		}
		o.Span = Span{Pos: t.Pos, End: p.lastEnd}
		return o
	case "annotation":
		target, ok := p.expectIdent("annotation target (app, entity, field or relation)")
		if !ok {
			return nil
		}
		open, ok := p.expect(LBrace, "after annotation target")
		if !ok {
			return nil
		}
		a := &AnnotationDecl{Doc: doc, Target: target}
		end := p.parseBlock(open, "annotation "+target.Name, func() bool {
			kdoc := p.takeDoc()
			name, ok := p.expectIdent("annotation key name")
			if !ok {
				return false
			}
			typ, ok := p.expectIdent("type of key " + name.Name)
			if !ok {
				return false
			}
			k := &AnnotationKeyDecl{Doc: kdoc, Name: name, Type: typ}
			if !p.parsePlainModifiers(&k.Modifiers) {
				return false
			}
			k.Span = Span{Pos: name.Pos, End: p.lastEnd}
			a.Keys = append(a.Keys, k)
			return true
		})
		a.Span = Span{Pos: t.Pos, End: end}
		return a
	case "slot":
		v, ok := p.expectString("slot name")
		if !ok {
			return nil
		}
		return &SlotItem{Doc: doc, Name: v, Pos: t.Pos, End: v.End}
	case "go_require":
		mod, ok := p.expectString("Go module path")
		if !ok {
			return nil
		}
		ver, ok := p.expectString("module version")
		if !ok {
			return nil
		}
		return &GoRequireItem{Module: mod, Version: ver, Pos: t.Pos, End: ver.End}
	case "hook":
		event, ok := p.expectIdent("hook event (pre-gen or post-gen)")
		if !ok {
			return nil
		}
		path, ok := p.expectString("hook path")
		if !ok {
			return nil
		}
		h := &HookItem{Event: event, Path: path, Pos: t.Pos, End: path.End}
		if w := p.tok(); w.Kind == Name && w.Text == "writes" {
			p.advance()
			v, ok := p.parseValue("after writes")
			if !ok {
				return nil
			}
			h.Writes = &v
			h.End = v.End
		}
		return h
	}
	p.errorf(t, "unknown manifest setting %q", t.Text).Suggestion = didYouMean(t.Text, manifestSettings)
	return nil
}

// parsePlainModifiers parses modifiers and rejects annotations, which make no
// sense on option and key declarations.
func (p *parser) parsePlainModifiers(mods *[]*Modifier) bool {
	var anns []*Annotation
	if !p.parseModifiers(mods, &anns) {
		return false
	}
	if len(anns) > 0 {
		p.diags.Errorf(anns[0].Pos, anns[0].End, "annotations are not allowed here")
		return false
	}
	return true
}
