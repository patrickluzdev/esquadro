package project

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// Source is the name and contents of one .esq file.
type Source struct {
	Name string
	Src  []byte
}

// Load parses and builds a project made of one or more .esq files, which
// together form a single project: declarations can reference each other
// across files and their order does not matter. Semantic checks only run when
// every file parses, to avoid reporting problems caused by a broken tree.
func Load(sources ...Source) (*Project, diag.List) {
	var diags diag.List
	files := make([]*dsl.File, 0, len(sources))
	for _, s := range sources {
		f, d := dsl.Parse(s.Name, s.Src)
		diags = append(diags, d...)
		files = append(files, f)
	}
	if diags.HasErrors() {
		diags.Sort()
		return nil, diags
	}
	p, more := Build(files...)
	diags = append(diags, more...)
	diags.Sort()
	return p, diags
}

// Build resolves and validates parsed files that form one project. It returns
// the project even when there are errors, so callers can inspect what was
// understood.
func Build(files ...*dsl.File) (*Project, diag.List) {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	b := &builder{
		files:    files,
		p:        &Project{Files: names},
		decls:    map[string]dsl.Decl{},
		entities: map[string]*Entity{},
		enums:    map[string]*Enum{},
	}
	b.build()
	b.diags.Sort()
	return b.p, b.diags
}

type builder struct {
	files    []*dsl.File
	p        *Project
	diags    diag.List
	decls    map[string]dsl.Decl // entity and enum names as written
	entities map[string]*Entity
	enums    map[string]*Enum
	// entityDecls keeps declaration order for the relation pass.
	entityDecls []*dsl.EntityDecl
	// explicitInverse holds inverse(name) modifiers until inverses are built.
	explicitInverse []pendingInverse
}

func (b *builder) errorf(s dsl.Span, format string, args ...any) *diag.Diagnostic {
	return b.diags.Errorf(s.Pos, s.End, format, args...)
}

// allDecls returns the declarations of every file, in file order.
func (b *builder) allDecls() []dsl.Decl {
	var out []dsl.Decl
	for _, f := range b.files {
		out = append(out, f.Decls...)
	}
	return out
}

// where describes ref for a message about at: "line 12" in the same file,
// "catalog.esq:12" in another one.
func where(at, ref diag.Pos) string {
	if at.File == ref.File {
		return fmt.Sprintf("line %d", ref.Line)
	}
	return fmt.Sprintf("%s:%d", ref.File, ref.Line)
}

func didYouMean(name string, cands []string) string {
	if s := diag.Suggest(name, cands); s != "" {
		return `did you mean "` + s + `"?`
	}
	return ""
}

// names builds Names for an identifier, reporting non-ASCII names.
func (b *builder) names(id dsl.Ident) Names {
	n, ok := makeNames(id.Name)
	if !ok {
		b.errorf(id.Span, "%q must use ASCII letters, digits and underscores only", id.Name).
			Suggestion = "names become Go identifiers, SQL columns and file paths"
		n, _ = makeNames(asciiOnly(id.Name))
		n.Raw = id.Name
	}
	return n
}

func asciiOnly(s string) string {
	var r []rune
	for _, c := range s {
		if c < utf8.RuneSelf {
			r = append(r, c)
		} else {
			r = append(r, '_')
		}
	}
	return string(r)
}

func (b *builder) build() {
	var apps []*dsl.AppDecl
	for _, d := range b.allDecls() {
		switch d := d.(type) {
		case *dsl.AppDecl:
			apps = append(apps, d)
		case *dsl.EnumDecl:
			b.declare(d.Name, d)
		case *dsl.EntityDecl:
			b.declare(d.Name, d)
		case *dsl.BlueprintDecl:
			b.errorf(d.Name.Span, "blueprint manifests cannot be part of a project").
				Suggestion = "put this block in blueprint.esq inside the blueprint's own folder"
		}
	}

	switch len(apps) {
	case 0:
		var start diag.Pos
		if len(b.files) > 0 {
			start = diag.Pos{File: b.files[0].Name, Line: 1, Col: 1}
		}
		msg := "missing app block"
		if len(b.files) > 1 {
			msg = "missing app block (none of the project's files has one)"
		}
		b.diags.Errorf(start, diag.Pos{}, "%s", msg).
			Suggestion = `add one, e.g. app shop { module "github.com/you/shop" }`
		b.p.App = &App{Pos: start}
	default:
		for _, extra := range apps[1:] {
			b.errorf(extra.Name.Span, "only one app block is allowed; the first one is at %s", where(extra.Name.Pos, apps[0].Pos))
		}
		b.p.App = b.buildApp(apps[0])
	}

	// Pass 1: enums, then entity shells and fields, so relations can point
	// at any entity and display any field regardless of declaration order.
	for _, d := range b.allDecls() {
		if d, ok := d.(*dsl.EnumDecl); ok && b.decls[d.Name.Name] == dsl.Decl(d) {
			b.buildEnum(d)
		}
	}
	for _, d := range b.allDecls() {
		if d, ok := d.(*dsl.EntityDecl); ok && b.decls[d.Name.Name] == dsl.Decl(d) {
			b.buildEntity(d)
		}
	}
	// Pass 2: declared relations.
	for _, d := range b.entityDecls {
		b.buildRelations(b.entities[d.Name.Name], d)
	}
	// Pass 3: inverse sides, then name clashes.
	b.inferInverses()
	b.checkTypeNameClashes()
	for _, e := range b.p.Entities {
		b.checkMemberClashes(e)
	}
}

// declare registers an entity or enum name, reporting duplicates and names
// that shadow built-in types.
func (b *builder) declare(name dsl.Ident, d dsl.Decl) {
	for _, t := range builtinTypes {
		if string(t) == name.Name {
			b.errorf(name.Span, "%q is a built-in type and cannot be used as a name", name.Name)
			return
		}
	}
	if prev, ok := b.decls[name.Name]; ok {
		b.errorf(name.Span, "%s is already declared at %s", name.Name, where(name.Pos, prev.SourceSpan().Pos))
		return
	}
	b.decls[name.Name] = d
}

// ---- app ----

func (b *builder) buildApp(d *dsl.AppDecl) *App {
	app := &App{Names: b.names(d.Name), Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}
	if d.Module == nil {
		b.errorf(d.Name.Span, "app %s has no module", d.Name.Name).
			Suggestion = `add the Go module path, e.g. module "github.com/you/` + app.Kebab + `"`
	} else {
		app.Module = d.Module.Text
		if app.Module == "" || strings.ContainsAny(app.Module, " \t\\") || strings.HasPrefix(app.Module, "/") || strings.HasSuffix(app.Module, "/") {
			b.errorf(d.Module.Span, "%q is not a valid Go module path", app.Module).
				Suggestion = `e.g. module "github.com/you/` + app.Kebab + `"`
		}
	}

	seen := map[string]*BlueprintRef{}
	for _, r := range d.Blueprints {
		ref := &BlueprintRef{Ref: r.Ref.Text, Options: r.Options, Pos: r.Pos, RefSpan: r.Ref.Span}
		switch {
		case r.Ref.Kind == dsl.IdentValue:
			ref.Source = SourceBuiltin
		case strings.HasPrefix(ref.Ref, "./") || strings.HasPrefix(ref.Ref, "../") || strings.HasPrefix(ref.Ref, "/"):
			ref.Source = SourceLocal
		case strings.Contains(ref.Ref, "/"):
			ref.Source = SourceGit
		default:
			b.errorf(r.Ref.Span, "%q is not a local path or a git source", ref.Ref).
				Suggestion = "built-in blueprints are written without quotes, e.g. blueprint " + ref.Ref +
				`; local ones start with "./"`
			continue
		}
		if prev, ok := seen[ref.Ref]; ok {
			b.errorf(r.Ref.Span, "blueprint %s is already used at line %d", ref.Ref, prev.Pos.Line)
			continue
		}
		seen[ref.Ref] = ref
		keys := map[string]*dsl.Option{}
		for _, o := range r.Options {
			if prev, ok := keys[o.Key.Name]; ok {
				b.errorf(o.Key.Span, "option %s is already set at line %d", o.Key.Name, prev.Pos.Line)
			}
			keys[o.Key.Name] = o
		}
		app.Blueprints = append(app.Blueprints, ref)
	}
	if len(d.Blueprints) == 0 {
		b.diags.Warnf(d.Name.Pos, d.Name.End, "app %s uses no blueprints, so nothing will be generated", d.Name.Name).
			Suggestion = "add one, e.g. blueprint platform"
	}
	return app
}

// ---- enums ----

func (b *builder) buildEnum(d *dsl.EnumDecl) {
	e := &Enum{Names: b.names(d.Name), Doc: d.Doc, Pos: d.Pos}
	if len(d.Values) == 0 {
		b.errorf(d.Name.Span, "enum %s has no values", d.Name.Name)
	}
	bySnake := map[string]*dsl.EnumValue{}
	for _, v := range d.Values {
		n := b.names(v.Name)
		if prev, ok := bySnake[n.Snake]; ok {
			if prev.Name.Name == v.Name.Name {
				b.errorf(v.Name.Span, "%s is already a value of %s", v.Name.Name, d.Name.Name)
			} else {
				b.errorf(v.Name.Span, "%s and %s (line %d) would get the same generated name %q", v.Name.Name, prev.Name.Name, prev.Name.Pos.Line, n.Snake)
			}
			continue
		}
		bySnake[n.Snake] = v
		e.Values = append(e.Values, &EnumValue{Names: n, Doc: v.Doc})
	}
	b.enums[d.Name.Name] = e
	b.p.Enums = append(b.p.Enums, e)
}

// ---- entities and fields ----

var entityOptions = []string{"plural"}

var (
	implicitID        = "id"
	implicitCreatedAt = "created_at"
	implicitUpdatedAt = "updated_at"
)

func implicitField(name string, t Type, d *Default) *Field {
	n, _ := makeNames(name)
	return &Field{Names: n, Type: t, Required: true, Implicit: true, Default: d}
}

func (b *builder) buildEntity(d *dsl.EntityDecl) {
	e := &Entity{Names: b.names(d.Name), Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}
	e.Plural, _ = makePlural(e.Raw)

	seenOpt := map[string]*dsl.Option{}
	for _, o := range d.Options {
		if prev, ok := seenOpt[o.Key.Name]; ok {
			b.errorf(o.Key.Span, "option %s is already set at line %d", o.Key.Name, prev.Pos.Line)
			continue
		}
		seenOpt[o.Key.Name] = o
		switch o.Key.Name {
		case "plural":
			if o.Value.Kind != dsl.StringValue || o.Value.Text == "" {
				b.errorf(o.Value.Span, "plural must be a non-empty string").Suggestion = `e.g. (plural "people")`
				continue
			}
			p, ok := makeNames(o.Value.Text)
			if !ok {
				b.errorf(o.Value.Span, "plural %q must use ASCII letters, digits and underscores only", o.Value.Text)
				continue
			}
			e.Plural = p
		default:
			b.errorf(o.Key.Span, "unknown entity option %q", o.Key.Name).Suggestion = didYouMean(o.Key.Name, entityOptions)
		}
	}

	e.Fields = append(e.Fields, implicitField(implicitID, TypeUUID, nil))
	for _, f := range d.Fields {
		if fld := b.buildField(f); fld != nil {
			e.Fields = append(e.Fields, fld)
		}
	}
	now := &Default{Kind: DefaultNow, Text: "now"}
	e.Fields = append(e.Fields,
		implicitField(implicitCreatedAt, TypeTime, now),
		implicitField(implicitUpdatedAt, TypeTime, now))

	b.entities[d.Name.Name] = e
	b.entityDecls = append(b.entityDecls, d)
	b.p.Entities = append(b.p.Entities, e)
}

var fieldModifiers = []string{"required", "unique", "min", "max", "pattern", "default"}

func (b *builder) buildField(d *dsl.Field) *Field {
	n := b.names(d.Name)
	switch n.Snake {
	case implicitID, implicitCreatedAt, implicitUpdatedAt:
		b.errorf(d.Name.Span, "%s is added to every entity automatically", n.Snake).
			Suggestion = "remove this field"
		return nil
	}
	f := &Field{Names: n, Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}

	typ := d.Type.Name
	switch {
	case isBuiltinType(typ):
		f.Type = Type(typ)
	case b.enums[typ] != nil:
		f.Type, f.Enum = TypeEnum, b.enums[typ]
	case b.isEntity(typ):
		b.errorf(d.Type.Span, "%s is an entity, not a field type", typ).
			Suggestion = fmt.Sprintf("use a relation: %s -> %s", d.Name.Name, typ)
		return nil
	default:
		cands := make([]string, 0, len(builtinTypes)+len(b.enums))
		for _, t := range builtinTypes {
			cands = append(cands, string(t))
		}
		for name := range b.enums {
			cands = append(cands, name)
		}
		e := b.errorf(d.Type.Span, "unknown type %q", typ)
		if s := didYouMean(typ, cands); s != "" {
			e.Suggestion = s
		} else {
			e.Suggestion = "built-in types are string, text, int, int64, decimal, bool, time, date, uuid and json"
		}
		return nil
	}

	seen := map[string]*dsl.Modifier{}
	var defaultMod *dsl.Modifier
	for _, m := range d.Modifiers {
		name := m.Name.Name
		if prev, ok := seen[name]; ok {
			b.errorf(m.Name.Span, "%s is already set at column %d", name, prev.Pos.Col)
			continue
		}
		seen[name] = m
		switch name {
		case "required":
			if b.noArgs(m) {
				f.Required = true
			}
		case "unique":
			if b.noArgs(m) {
				f.Unique = true
			}
		case "min", "max":
			f.setBound(b, m)
		case "pattern":
			b.setPattern(f, m)
		case "default":
			defaultMod = m
		case "inverse":
			b.errorf(m.Name.Span, "inverse is only allowed on relations")
		default:
			b.errorf(m.Name.Span, "unknown modifier %q", name).Suggestion = didYouMean(name, fieldModifiers)
		}
	}
	if f.Min != nil && f.Max != nil && f.Min.Value > f.Max.Value {
		m := seen["min"]
		b.errorf(m.Span, "min (%s) is greater than max (%s)", f.Min.Text, f.Max.Text)
	}
	if defaultMod != nil {
		b.setDefault(f, defaultMod)
	}
	return f
}

// article returns "an" before words starting with a, e, i or o, else "a".
func article(word string) string {
	if word != "" && strings.ContainsRune("aeioAEIO", rune(word[0])) {
		return "an"
	}
	return "a"
}

// nameEnd returns the end position of a name that starts at pos.
func nameEnd(pos diag.Pos, raw string) diag.Pos {
	end := pos
	end.Col += utf8.RuneCountInString(raw)
	end.Offset += len(raw)
	return end
}

func isBuiltinType(s string) bool {
	for _, t := range builtinTypes {
		if string(t) == s {
			return true
		}
	}
	return false
}

func (b *builder) isEntity(name string) bool {
	_, ok := b.decls[name].(*dsl.EntityDecl)
	return ok
}

func (b *builder) noArgs(m *dsl.Modifier) bool {
	if m.HasParens {
		b.errorf(m.Span, "%s takes no arguments", m.Name.Name).Suggestion = "write just " + m.Name.Name
		return false
	}
	return true
}

func (b *builder) oneArg(m *dsl.Modifier, example string) (dsl.Value, bool) {
	if !m.HasParens || len(m.Args) != 1 {
		b.errorf(m.Span, "%s needs exactly one value", m.Name.Name).Suggestion = "e.g. " + example
		return dsl.Value{}, false
	}
	return m.Args[0], true
}

func (f *Field) setBound(b *builder, m *dsl.Modifier) {
	name := m.Name.Name
	v, ok := b.oneArg(m, name+"(3)")
	if !ok {
		return
	}
	var integer bool
	switch f.Type {
	case TypeString, TypeText:
		integer = true
	case TypeInt, TypeInt64:
		integer = true
	case TypeDecimal:
	default:
		b.errorf(m.Name.Span, "%s is not allowed on %s fields", name, f.typeName()).
			Suggestion = "min and max apply to string, text, int, int64 and decimal"
		return
	}
	if v.Kind != dsl.NumberValue {
		b.errorf(v.Span, "%s must be a number", name)
		return
	}
	num, err := strconv.ParseFloat(v.Text, 64)
	if err != nil {
		b.errorf(v.Span, "%s is not a valid number", v.Text)
		return
	}
	if integer && strings.Contains(v.Text, ".") {
		b.errorf(v.Span, "%s on %s fields must be a whole number", name, f.typeName())
		return
	}
	if (f.Type == TypeString || f.Type == TypeText) && num < 0 {
		b.errorf(v.Span, "%s on %s fields is a length and cannot be negative", name, f.typeName())
		return
	}
	n := &Number{Text: v.Text, Value: num}
	if name == "min" {
		f.Min = n
	} else {
		f.Max = n
	}
}

func (f *Field) typeName() string {
	if f.Type == TypeEnum && f.Enum != nil {
		return f.Enum.Raw
	}
	return string(f.Type)
}

func (b *builder) setPattern(f *Field, m *dsl.Modifier) {
	v, ok := b.oneArg(m, "pattern(`^[a-z]+$`)")
	if !ok {
		return
	}
	if f.Type != TypeString && f.Type != TypeText {
		b.errorf(m.Name.Span, "pattern is not allowed on %s fields", f.typeName()).
			Suggestion = "pattern applies to string and text"
		return
	}
	if v.Kind != dsl.StringValue {
		b.errorf(v.Span, "pattern must be a string").Suggestion = "e.g. pattern(`^[a-z]+$`)"
		return
	}
	if _, err := regexp.Compile(v.Text); err != nil {
		b.errorf(v.Span, "invalid pattern: %v", strings.TrimPrefix(err.Error(), "error parsing regexp: "))
		return
	}
	f.Pattern = v.Text
}

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (b *builder) setDefault(f *Field, m *dsl.Modifier) {
	v, ok := b.oneArg(m, "default(0)")
	if !ok {
		return
	}
	bad := func(want, example string) {
		b.errorf(v.Span, "default for %s %s field must be %s", article(f.typeName()), f.typeName(), want).Suggestion = "e.g. default(" + example + ")"
	}
	lit := &Default{Kind: DefaultLiteral, Text: v.Text}
	switch f.Type {
	case TypeString, TypeText:
		if v.Kind != dsl.StringValue {
			bad("a string", `""`)
			return
		}
		runes := float64(utf8.RuneCountInString(v.Text))
		if f.Min != nil && runes < f.Min.Value || f.Max != nil && runes > f.Max.Value {
			b.errorf(v.Span, "default %q does not satisfy the field's min/max length", v.Text)
			return
		}
		if f.Pattern != "" && !regexp.MustCompile(f.Pattern).MatchString(v.Text) {
			b.errorf(v.Span, "default %q does not match the field's pattern", v.Text)
			return
		}
	case TypeJSON:
		if v.Kind != dsl.StringValue || !json.Valid([]byte(v.Text)) {
			bad("a string with valid JSON", `"{}"`)
			return
		}
	case TypeInt, TypeInt64, TypeDecimal:
		if v.Kind != dsl.NumberValue || (f.Type != TypeDecimal && strings.Contains(v.Text, ".")) {
			if f.Type == TypeDecimal {
				bad("a number", "0")
			} else {
				bad("a whole number", "0")
			}
			return
		}
		num, _ := strconv.ParseFloat(v.Text, 64)
		if f.Min != nil && num < f.Min.Value || f.Max != nil && num > f.Max.Value {
			b.errorf(v.Span, "default %s is outside the field's min/max", v.Text)
			return
		}
	case TypeBool:
		if v.Kind != dsl.BoolValue {
			bad("true or false", "false")
			return
		}
	case TypeTime, TypeDate:
		switch {
		case v.Kind == dsl.IdentValue && v.Text == "now":
			lit = &Default{Kind: DefaultNow, Text: "now"}
		case v.Kind == dsl.StringValue && f.Type == TypeDate:
			if _, err := time.Parse(time.DateOnly, v.Text); err != nil {
				bad(`now or a date like "2025-01-31"`, "now")
				return
			}
		case v.Kind == dsl.StringValue && f.Type == TypeTime:
			if _, err := time.Parse(time.RFC3339, v.Text); err != nil {
				bad(`now or a time like "2025-01-31T09:00:00Z"`, "now")
				return
			}
		default:
			bad("now or a quoted value", "now")
			return
		}
	case TypeUUID:
		if v.Kind != dsl.StringValue || !uuidRE.MatchString(v.Text) {
			bad("a quoted UUID", `"00000000-0000-0000-0000-000000000000"`)
			return
		}
	case TypeEnum:
		var names []string
		for _, ev := range f.Enum.Values {
			names = append(names, ev.Raw)
		}
		if v.Kind != dsl.IdentValue {
			bad("one of its values", strings.Join(names, " | "))
			return
		}
		ev := f.Enum.Value(v.Text)
		if ev == nil {
			e := b.errorf(v.Span, "%s is not a value of %s", v.Text, f.Enum.Raw)
			if s := didYouMean(v.Text, names); s != "" {
				e.Suggestion = s
			} else {
				e.Suggestion = "values are " + strings.Join(names, ", ")
			}
			return
		}
		lit = &Default{Kind: DefaultEnum, Text: v.Text, EnumValue: ev}
	}
	f.Default = lit
}

// ---- relations ----

var relationModifiers = []string{"required", "unique", "inverse"}

// pendingInverse remembers an explicit inverse(name) until pass 3.
type pendingInverse struct {
	rel  *Relation
	name *Names
	span dsl.Span
}

func (b *builder) buildRelations(e *Entity, d *dsl.EntityDecl) {
	for _, r := range d.Relations {
		rel := b.buildRelation(e, r)
		if rel != nil {
			e.Relations = append(e.Relations, rel)
		}
	}
}

func (b *builder) buildRelation(e *Entity, d *dsl.Relation) *Relation {
	target := b.entities[d.Target.Name]
	if target == nil {
		switch {
		case b.enums[d.Target.Name] != nil:
			b.errorf(d.Target.Span, "%s is an enum, not an entity", d.Target.Name).
				Suggestion = fmt.Sprintf("use a field: %s %s", d.Name.Name, d.Target.Name)
		case isBuiltinType(d.Target.Name):
			b.errorf(d.Target.Span, "%s is a field type, not an entity", d.Target.Name).
				Suggestion = fmt.Sprintf("use a field: %s %s", d.Name.Name, d.Target.Name)
		case b.isEntity(d.Target.Name):
			// Declared but failed to build; already reported.
		default:
			var names []string
			for name, decl := range b.decls {
				if _, ok := decl.(*dsl.EntityDecl); ok {
					names = append(names, name)
				}
			}
			b.errorf(d.Target.Span, "unknown entity %q", d.Target.Name).Suggestion = didYouMean(d.Target.Name, names)
		}
		return nil
	}

	rel := &Relation{
		Names: b.names(d.Name), Doc: d.Doc, Entity: e, Target: target,
		Annotations: d.Annotations, Pos: d.Pos,
	}
	if d.Kind == dsl.ManyToMany {
		rel.Kind = ManyToMany
		rel.JoinTable = e.Snake + "_" + rel.Snake
	}

	seen := map[string]bool{}
	for _, m := range d.Modifiers {
		name := m.Name.Name
		if seen[name] {
			b.errorf(m.Name.Span, "%s is already set", name)
			continue
		}
		seen[name] = true
		switch name {
		case "required", "unique":
			if rel.Kind == ManyToMany {
				b.errorf(m.Name.Span, "%s is not allowed on many-to-many relations", name)
				continue
			}
			if !b.noArgs(m) {
				continue
			}
			if name == "required" {
				rel.Required = true
			} else {
				rel.Kind = OneToOne
			}
		case "inverse":
			v, ok := b.oneArg(m, "inverse(products)")
			if !ok {
				continue
			}
			if v.Kind != dsl.IdentValue && v.Kind != dsl.StringValue {
				b.errorf(v.Span, "inverse must be a name").Suggestion = "e.g. inverse(products)"
				continue
			}
			n := b.names(dsl.Ident{Name: v.Text, Span: v.Span})
			b.explicitInverse = append(b.explicitInverse, pendingInverse{rel: rel, name: &n, span: v.Span})
		case "min", "max", "pattern", "default":
			b.errorf(m.Name.Span, "%s is not allowed on relations", name)
		default:
			b.errorf(m.Name.Span, "unknown modifier %q", name).Suggestion = didYouMean(name, relationModifiers)
		}
	}

	if d.Display != nil {
		f := target.Field(d.Display.Name)
		if f == nil {
			e := b.errorf(d.Display.Span, "%s has no field %q to display", target.Raw, d.Display.Name)
			var names []string
			for _, tf := range target.Fields {
				names = append(names, tf.Raw)
			}
			e.Suggestion = didYouMean(d.Display.Name, names)
		}
		rel.Display = f
	} else {
		rel.Display = defaultDisplay(target)
	}
	return rel
}

// defaultDisplay picks the field shown for an entity when another entity
// points to it: name, then title, then the first string field, then id.
func defaultDisplay(e *Entity) *Field {
	for _, n := range []string{"name", "title"} {
		if f := e.Field(n); f != nil {
			return f
		}
	}
	for _, f := range e.Fields {
		if f.Type == TypeString && !f.Implicit {
			return f
		}
	}
	return e.Fields[0] // id
}

// inferInverses adds the inverse side of every declared relation to its
// target. The inverse is named after the source entity (plural for
// one-to-many and many-to-many: Category.products), or after the source and
// the relation when that would be ambiguous (Person.people_by_manager), unless
// inverse(name) was given.
func (b *builder) inferInverses() {
	explicit := map[*Relation]pendingInverse{}
	for _, pi := range b.explicitInverse {
		explicit[pi.rel] = pi
	}
	count := map[[2]*Entity]int{}
	for _, e := range b.p.Entities {
		for _, r := range e.Relations {
			count[[2]*Entity{e, r.Target}]++
		}
	}
	for _, e := range b.p.Entities {
		declared := append([]*Relation(nil), e.Relations...)
		for _, r := range declared {
			if r.Inferred {
				continue
			}
			kind := r.Kind
			from := e.Plural
			switch r.Kind {
			case ManyToOne:
				kind = OneToMany
			case OneToOne:
				from = e.Names
			}
			var n Names
			if pi, ok := explicit[r]; ok {
				n = *pi.name
			} else if count[[2]*Entity{e, r.Target}] > 1 || e == r.Target {
				n, _ = makeNames(from.Snake + "_by_" + r.Snake)
			} else {
				n, _ = makeNames(from.Snake)
			}
			inv := &Relation{
				Names: n, Kind: kind, Entity: r.Target, Target: e,
				Display: defaultDisplay(e), Inverse: r, Inferred: true,
				JoinTable: r.JoinTable, Pos: r.Pos,
			}
			r.Inverse = inv
			r.Target.Relations = append(r.Target.Relations, inv)
		}
	}
}

// ---- clashes ----

func (b *builder) checkTypeNameClashes() {
	type owner struct {
		raw string
		pos diag.Pos
	}
	pascal := map[string]owner{}
	check := func(n Names, pos diag.Pos, span dsl.Span) {
		if prev, ok := pascal[n.Pascal]; ok && prev.raw != n.Raw {
			b.errorf(span, "%s and %s (%s) would get the same generated name %q", n.Raw, prev.raw, where(span.Pos, prev.pos), n.Pascal)
			return
		}
		pascal[n.Pascal] = owner{n.Raw, pos}
	}
	tables := map[string]*Entity{}
	for _, d := range b.allDecls() {
		switch d := d.(type) {
		case *dsl.EnumDecl:
			if e := b.enums[d.Name.Name]; e != nil {
				check(e.Names, e.Pos, d.Name.Span)
			}
		case *dsl.EntityDecl:
			e := b.entities[d.Name.Name]
			if e == nil {
				continue
			}
			check(e.Names, e.Pos, d.Name.Span)
			if prev, ok := tables[e.Plural.Snake]; ok && prev != e {
				b.errorf(d.Name.Span, "%s and %s (%s) have the same plural %q", e.Raw, prev.Raw, where(d.Name.Pos, prev.Pos), e.Plural.Snake).
					Suggestion = `set a different plural with (plural "...")`
				continue
			}
			tables[e.Plural.Snake] = e
		}
	}
}

// checkMemberClashes reports fields and relations of one entity that would
// get the same generated name, including implicit fields, inverse relations
// and foreign key columns (category -> Category uses category_id).
func (b *builder) checkMemberClashes(e *Entity) {
	type member struct {
		what string
		raw  string
		pos  diag.Pos
	}
	seen := map[string]member{}
	for _, f := range e.Fields {
		if prev, ok := seen[f.Snake]; ok {
			if prev.raw == f.Raw {
				b.diags.Errorf(f.Pos, nameEnd(f.Pos, f.Raw), "field %s is already declared at line %d", f.Raw, prev.pos.Line)
			} else {
				b.diags.Errorf(f.Pos, nameEnd(f.Pos, f.Raw), "fields %s and %s (line %d) would get the same generated name %q", f.Raw, prev.raw, prev.pos.Line, f.Snake)
			}
			continue
		}
		seen[f.Snake] = member{"field", f.Raw, f.Pos}
	}
	for _, r := range e.Relations {
		if r.Inferred {
			continue
		}
		if prev, ok := seen[r.Snake]; ok {
			b.diags.Errorf(r.Pos, nameEnd(r.Pos, r.Raw), "relation %s clashes with %s %s (line %d)", r.Raw, prev.what, prev.raw, prev.pos.Line)
			continue
		}
		seen[r.Snake] = member{"relation", r.Raw, r.Pos}
		if r.OwnsForeignKey() {
			fk := r.Snake + "_id"
			if prev, ok := seen[fk]; ok {
				b.diags.Errorf(prev.pos, nameEnd(prev.pos, prev.raw), "%s %s clashes with the foreign key column %s of relation %s", prev.what, prev.raw, fk, r.Raw).
					Suggestion = "remove it: the relation already stores the key"
				continue
			}
			seen[fk] = member{"foreign key of", r.Raw, r.Pos}
		}
	}
	for _, r := range e.Relations {
		if !r.Inferred {
			continue
		}
		if prev, ok := seen[r.Snake]; ok {
			src := r.Inverse
			b.diags.Errorf(src.Pos, nameEnd(src.Pos, src.Raw), "the inverse of %s.%s would be %s.%s, which clashes with %s %s (%s)",
				src.Entity.Raw, src.Raw, e.Raw, r.Snake, prev.what, prev.raw, where(src.Pos, prev.pos)).
				Suggestion = "name the inverse yourself, e.g. inverse(" + r.Snake + "_list)"
			continue
		}
		seen[r.Snake] = member{"relation", r.Raw, r.Pos}
	}
}
