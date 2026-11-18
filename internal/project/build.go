package project

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bojanz/currency"

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
	// oneToMany holds the "<" relations until they are paired.
	oneToMany []pendingOneToMany
	// modules by name, and the module of each entity and enum declaration.
	modules  map[string]*Module
	moduleOf map[dsl.Decl]*Module
	jobs     map[string]*Job
	reports  map[string]*Report
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
	var apps, platforms, servers []*dsl.AppDecl
	for _, d := range b.allDecls() {
		if d, ok := d.(*dsl.AppDecl); ok {
			switch d.Kind() {
			case "platform":
				platforms = append(platforms, d)
			case "server":
				servers = append(servers, d)
			default:
				apps = append(apps, d)
			}
		}
	}
	if len(platforms) > 0 {
		b.buildPlatform(platforms, servers, apps)
		return
	}
	b.p.Kind = KindApp
	for _, s := range servers {
		b.errorf(s.Name.Span, "server %s belongs to a platform project", s.Name.Name).
			Suggestion = "servers are declared in a project made with esquadro new -starter platform"
	}
	for _, d := range b.allDecls() {
		switch d := d.(type) {
		case *dsl.AppDecl:
		case *dsl.EnumDecl:
			b.declare(d.Name, d)
		case *dsl.EntityDecl:
			b.declare(d.Name, d)
		case *dsl.JobDecl:
			b.declare(d.Name, d)
		case *dsl.ReportDecl:
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

	b.buildModules()

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
	for _, d := range b.allDecls() {
		if d, ok := d.(*dsl.JobDecl); ok && b.decls[d.Name.Name] == dsl.Decl(d) {
			b.buildJob(d)
		}
	}
	for _, d := range b.allDecls() {
		if d, ok := d.(*dsl.ReportDecl); ok && b.decls[d.Name.Name] == dsl.Decl(d) {
			b.buildReport(d)
		}
	}
	// Pass 2: declared relations.
	for _, d := range b.entityDecls {
		b.buildRelations(b.entities[d.Name.Name], d)
	}
	// Pass 3: inverse sides, then name clashes.
	b.pairOneToMany()
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
	if d.Kind() == "app" {
		var currencyAt *dsl.Setting
		for _, s := range d.Settings {
			if s.Key.Name == "currency" {
				if currencyAt != nil {
					b.errorf(s.Key.Span, "currency is already set at line %d", currencyAt.Pos.Line)
					continue
				}
				currencyAt = s
				b.setCurrency(app, s)
				continue
			}
			b.errorf(s.Key.Span, "unknown app setting %q", s.Key.Name).Suggestion = didYouMean(s.Key.Name, appSettings)
		}
	}
	switch {
	case d.Kind() == "platform":
		// A platform has no Go module.
	case d.Module == nil:
		b.errorf(d.Name.Span, "app %s has no module", d.Name.Name).
			Suggestion = `add the Go module path, e.g. module "github.com/you/` + app.Kebab + `"`
	default:
		app.Module = d.Module.Text
		if app.Module == "" || strings.ContainsAny(app.Module, " \t\\") || strings.HasPrefix(app.Module, "/") || strings.HasSuffix(app.Module, "/") {
			b.errorf(d.Module.Span, "%q is not a valid Go module path", app.Module).
				Suggestion = `e.g. module "github.com/you/` + app.Kebab + `"`
		}
	}

	for _, it := range d.Items {
		e, ok := it.(*dsl.EnvBlock)
		if !ok {
			continue
		}
		if e.Environment == nil {
			for _, decl := range e.Decls {
				if v := EnvVarOf(decl, func(s dsl.Span, format string, args ...any) { b.errorf(s, format, args...) }); v != nil {
					app.Env = append(app.Env, v)
				}
			}
			continue
		}
		for _, o := range e.Values {
			app.EnvValues = append(app.EnvValues, EnvValue{Environment: *e.Environment, Name: o.Key, Value: o.Value})
		}
	}

	seen := map[string]*BlueprintRef{}
	for _, r := range d.Blueprints {
		ref := &BlueprintRef{Ref: r.Ref.Text, Options: r.Options, Pos: r.Pos, RefSpan: r.Ref.Span}
		if r.Instance != nil {
			ref.Instance, ref.InstanceSpan = r.Instance.Name, r.Instance.Span
		}
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
		key := ref.Ref + " " + ref.Instance
		if prev, ok := seen[key]; ok {
			b.errorf(r.Ref.Span, "blueprint %s is already used at line %d", strings.TrimSpace(key), prev.Pos.Line)
			continue
		}
		seen[key] = ref
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
		b.diags.Warnf(d.Name.Pos, d.Name.End, d.Kind()+" %s uses no blueprints, so nothing will be generated", d.Name.Name).
			Suggestion = "add one, e.g. blueprint app; see them with esquadro blueprint list"
	}
	return app
}

// ---- enums ----

func (b *builder) buildEnum(d *dsl.EnumDecl) {
	e := &Enum{Names: b.names(d.Name), Module: b.moduleOf[d], Doc: d.Doc, Pos: d.Pos}
	if e.Module != nil {
		e.Module.Enums = append(e.Module.Enums, e)
	}
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
		e.Values = append(e.Values, &EnumValue{Names: n, Doc: v.Doc, Annotations: v.Annotations, Pos: v.Name.Pos})
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
	implicitVersion   = "version"
)

// buildJob builds a job: its arguments are fields, without the implicit
// ones; it has no relations, and nothing is unique in a queue.
func (b *builder) buildJob(d *dsl.JobDecl) {
	j := &Job{Names: b.names(d.Name), Module: b.moduleOf[d], Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}
	if j.Module != nil {
		j.Module.Jobs = append(j.Module.Jobs, j)
	}
	for _, r := range d.Relations {
		b.errorf(r.Name.Span, "a job's arguments cannot be relations: they are stored in the queue, as values").
			Suggestion = fmt.Sprintf("pass the id instead: %s_id uuid", strings.TrimSuffix(b.names(r.Name).Snake, "_id"))
	}
	for _, f := range d.Fields {
		for _, m := range f.Modifiers {
			if m.Name.Name == "unique" {
				b.errorf(m.Name.Span, "unique is for entities: a job's arguments are not stored in a table").
					Suggestion = "remove it"
			}
		}
		if fld := b.buildField(j.Module, f, false); fld != nil {
			j.Fields = append(j.Fields, fld)
		}
	}
	seen := map[string]*Field{}
	for _, f := range j.Fields {
		if prev, ok := seen[f.Snake]; ok {
			b.errorf(dsl.Span{Pos: f.Pos, End: nameEnd(f.Pos, f.Raw)}, "%s is already an argument of %s, at line %d", f.Raw, j.Raw, prev.Pos.Line)
			continue
		}
		seen[f.Snake] = f
	}
	if b.jobs == nil {
		b.jobs = map[string]*Job{}
	}
	b.jobs[d.Name.Name] = j
	b.p.Jobs = append(b.p.Jobs, j)
}

// buildReport builds a report: its fields are the columns of its rows,
// those marked param the parameters a request passes; no relations, as a
// row holds values, and nothing unique or with a default.
func (b *builder) buildReport(d *dsl.ReportDecl) {
	r := &Report{Names: b.names(d.Name), Module: b.moduleOf[d], Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}
	if r.Module != nil {
		r.Module.Reports = append(r.Module.Reports, r)
	}
	for _, rel := range d.Relations {
		b.errorf(rel.Name.Span, "a report's columns cannot be relations: its rows hold values").
			Suggestion = fmt.Sprintf("select the id instead: %s_id uuid", strings.TrimSuffix(b.names(rel.Name).Snake, "_id"))
	}
	seen := map[string]*Field{}
	for _, f := range d.Fields {
		param := false
		mods := make([]*dsl.Modifier, 0, len(f.Modifiers))
		for _, m := range f.Modifiers {
			switch m.Name.Name {
			case "param":
				if b.noArgs(m) {
					param = true
				}
				continue
			case "unique", "default":
				b.errorf(m.Name.Span, "%s is for entities: a report's columns and parameters are values its SQL works with", m.Name.Name).
					Suggestion = "remove it"
				continue
			}
			mods = append(mods, m)
		}
		field := *f
		field.Modifiers = mods
		fld := b.buildField(r.Module, &field, false)
		if fld == nil {
			continue
		}
		if prev, ok := seen[fld.Snake]; ok {
			b.errorf(dsl.Span{Pos: fld.Pos, End: nameEnd(fld.Pos, fld.Raw)}, "%s is already a column or parameter of %s, at line %d", fld.Raw, r.Raw, prev.Pos.Line)
			continue
		}
		seen[fld.Snake] = fld
		if fld.Type == TypeJSON {
			b.errorf(dsl.Span{Pos: fld.Pos, End: nameEnd(fld.Pos, fld.Raw)}, "a report's columns and parameters are plain values: json is not one").
				Suggestion = "select the values the report shows, each in a column"
			continue
		}
		if param {
			r.Params = append(r.Params, fld)
		} else {
			r.Columns = append(r.Columns, fld)
		}
	}
	if len(r.Columns) == 0 {
		b.errorf(d.Name.Span, "report %s has no columns: what does each row hold?", r.Raw).
			Suggestion = "declare its columns as fields, e.g. clicks int required"
	}
	if b.reports == nil {
		b.reports = map[string]*Report{}
	}
	b.reports[d.Name.Name] = r
	b.p.Reports = append(b.p.Reports, r)
}

func implicitField(name string, t Type, d *Default) *Field {
	n, _ := makeNames(name)
	return &Field{Names: n, Type: t, Required: true, Implicit: true, Default: d}
}

func (b *builder) buildEntity(d *dsl.EntityDecl) {
	e := &Entity{Names: b.names(d.Name), Module: b.moduleOf[d], Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}
	if e.Module != nil {
		e.Module.Entities = append(e.Module.Entities, e)
	}
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
		if fld := b.buildField(e.Module, f, true); fld != nil {
			e.Fields = append(e.Fields, fld)
		}
	}
	now := &Default{Kind: DefaultNow, Text: "now"}
	e.Fields = append(e.Fields,
		// version goes up with each change: the API's ETag, and what
		// If-Match checks.
		implicitField(implicitVersion, TypeInt, &Default{Kind: DefaultLiteral, Text: "1"}),
		implicitField(implicitCreatedAt, TypeTime, now),
		implicitField(implicitUpdatedAt, TypeTime, now))

	b.entities[d.Name.Name] = e
	b.entityDecls = append(b.entityDecls, d)
	b.p.Entities = append(b.p.Entities, e)
}

var fieldModifiers = []string{"required", "unique", "min", "max", "pattern", "default"}

// buildField builds a field of an entity, or with entity false an argument
// of a job, which has no implicit fields.
func (b *builder) buildField(mod *Module, d *dsl.Field, entity bool) *Field {
	n := b.names(d.Name)
	switch n.Snake {
	case implicitID, implicitCreatedAt, implicitUpdatedAt, implicitVersion:
		if entity {
			b.errorf(d.Name.Span, "%s is added to every entity automatically", n.Snake).
				Suggestion = "remove this field"
			return nil
		}
	}
	f := &Field{Names: n, Doc: d.Doc, Annotations: d.Annotations, Pos: d.Pos}

	typ := d.Type.Name
	switch {
	case isBuiltinType(typ):
		f.Type = Type(typ)
	case b.enums[typ] != nil:
		f.Type, f.Enum = TypeEnum, b.enums[typ]
		if em := f.Enum.Module; em != nil && mod != nil && em != mod {
			b.errorf(d.Type.Span, "enum %s belongs to module %s; an enum is used only inside its own module", typ, em.Raw).
				Suggestion = "declare the enum in module " + mod.Raw + " too, with the values this module needs"
			return nil
		}
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
			e.Suggestion = "built-in types are string, text, int, int64, decimal, money, bool, time, date, uuid and json"
		}
		return nil
	}
	if f.Type == TypeMoney && b.p.App != nil && b.p.App.Currency == "" && b.p.Kind == KindApp {
		b.errorf(d.Type.Span, "money needs the app's currency").
			Suggestion = "add it to the app block, e.g. currency BRL"
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
			if f.Type == TypeMoney {
				b.errorf(m.Name.Span, "unique is not allowed on money fields").
					Suggestion = "an amount is rarely an identity; make another field unique"
				continue
			}
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

// setCurrency sets the app's currency: "currency BRL", an ISO 4217 code.
func (b *builder) setCurrency(app *App, s *dsl.Setting) {
	if s.Value == nil || (s.Value.Kind != dsl.IdentValue && s.Value.Kind != dsl.StringValue) {
		b.errorf(s.Key.Span, "currency needs an ISO 4217 code").Suggestion = "e.g. currency BRL"
		return
	}
	code := s.Value.Text
	if !currency.IsValid(code) {
		e := b.errorf(s.Value.Span, "%q is not an ISO 4217 currency code", code)
		e.Suggestion = "e.g. currency BRL, currency USD, currency EUR"
		if up := strings.ToUpper(code); up != code && currency.IsValid(up) {
			e.Suggestion = "currency " + up
		}
		return
	}
	app.Currency = code
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
	case TypeDecimal, TypeMoney:
	default:
		b.errorf(m.Name.Span, "%s is not allowed on %s fields", name, f.typeName()).
			Suggestion = "min and max apply to string, text, int, int64, decimal and money"
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
	case TypeInt, TypeInt64, TypeDecimal, TypeMoney:
		exact := f.Type == TypeDecimal || f.Type == TypeMoney
		if v.Kind != dsl.NumberValue || (!exact && strings.Contains(v.Text, ".")) {
			if exact {
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

var (
	relationModifiers = []string{"required", "on_delete"}
	onDeleteActions   = []string{"cascade", "restrict", "set_null"}
)

// pendingOneToMany remembers a "<" until every entity's relations are built,
// so it can be paired with the ">" on the other side.
type pendingOneToMany struct {
	rel   *Relation
	field *dsl.Ident // the relation named after "." on the target, if any
	span  dsl.Span
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
	switch d.Kind {
	case dsl.ManyToOne:
		rel.Kind = ManyToOne
	case dsl.OneToOne:
		rel.Kind = OneToOne
	case dsl.ManyToMany:
		rel.Kind = ManyToMany
		rel.JoinTable = e.Snake + "_" + rel.Snake
	case dsl.OneToMany:
		rel.Kind = OneToMany
		if e.Module != nil && target.Module != nil && e.Module != target.Module {
			b.errorf(d.Name.Span, "%s cannot declare the other side of a relation in module %s", e.Raw, target.Module.Raw).
				Suggestion = "a module does not know who points at it; declare the relation in " + target.Module.Raw + " with >"
			return nil
		}
		b.oneToMany = append(b.oneToMany, pendingOneToMany{rel: rel, field: d.TargetField, span: d.Span})
	}
	if d.TargetField != nil && d.Kind != dsl.OneToMany {
		b.errorf(d.TargetField.Span, "only < names a relation on the other side").
			Suggestion = "write just the entity: " + d.Name.Name + " " + d.Kind.String() + " " + d.Target.Name
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
		case "required", "on_delete":
			if rel.Kind != ManyToOne && rel.Kind != OneToOne {
				e := b.errorf(m.Name.Span, "%s is not allowed on %s relations", name, rel.Kind)
				if rel.Kind == OneToMany {
					e.Suggestion = "it belongs on the > side, which holds the key"
				}
				continue
			}
			if name == "required" {
				if b.noArgs(m) {
					rel.Required = true
				}
				continue
			}
			v, ok := b.oneArg(m, "on_delete(cascade)")
			if !ok {
				continue
			}
			if v.Kind != dsl.IdentValue || !slices.Contains(onDeleteActions, v.Text) {
				e := b.errorf(v.Span, "on_delete must be cascade, restrict or set_null")
				if s := didYouMean(v.Text, onDeleteActions); s != "" {
					e.Suggestion = s
				}
				continue
			}
			rel.OnDelete = v.Text
		case "unique":
			b.errorf(m.Name.Span, "unique is not a relation modifier").
				Suggestion = fmt.Sprintf("for one-to-one, write: %s - %s", d.Name.Name, d.Target.Name)
		case "inverse":
			b.errorf(m.Name.Span, "inverse is not a relation modifier").
				Suggestion = fmt.Sprintf("name the other side on %s with <, e.g. %s < %s", d.Target.Name, e.Plural.Snake, e.Raw)
		case "min", "max", "pattern", "default":
			b.errorf(m.Name.Span, "%s is not allowed on relations", name)
		default:
			b.errorf(m.Name.Span, "unknown modifier %q", name).Suggestion = didYouMean(name, relationModifiers)
		}
	}
	if rel.OnDelete == "set_null" && rel.Required {
		b.errorf(d.Span, "a required relation cannot use on_delete(set_null)").
			Suggestion = "use cascade or restrict, or drop required"
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

// pairOneToMany pairs each "<" with the ">" on the other side: the one it
// names after ".", or the only one pointing back. With none, it adds that
// side to the target, named after this entity, as if it were declared there.
func (b *builder) pairOneToMany() {
	for _, pending := range b.oneToMany {
		r := pending.rel
		var candidates []*Relation
		for _, other := range r.Target.Relations {
			if !other.Inferred && other.Kind == ManyToOne && other.Target == r.Entity {
				candidates = append(candidates, other)
			}
		}
		var pair *Relation
		switch {
		case pending.field != nil:
			for _, other := range r.Target.Relations {
				if other.Raw == pending.field.Name {
					pair = other
				}
			}
			if pair == nil || pair.Kind != ManyToOne || pair.Target != r.Entity || pair.Inferred {
				var names []string
				for _, c := range candidates {
					names = append(names, c.Raw)
				}
				e := b.errorf(pending.field.Span, "%s has no relation %s > %s", r.Target.Raw, pending.field.Name, r.Entity.Raw)
				e.Suggestion = didYouMean(pending.field.Name, names)
				continue
			}
		case len(candidates) == 1:
			pair = candidates[0]
		case len(candidates) > 1:
			var names []string
			for _, c := range candidates {
				names = append(names, c.Raw)
			}
			b.errorf(pending.span, "%s has several relations to %s (%s); say which one this is", r.Target.Raw, r.Entity.Raw, strings.Join(names, ", ")).
				Suggestion = fmt.Sprintf("e.g. %s < %s.%s", r.Raw, r.Target.Raw, names[0])
			continue
		default:
			n, _ := makeNames(r.Entity.Snake)
			pair = &Relation{
				Names: n, Kind: ManyToOne, Entity: r.Target, Target: r.Entity,
				Display: defaultDisplay(r.Entity), Inferred: true, Pos: r.Pos,
			}
			r.Target.Relations = append(r.Target.Relations, pair)
		}
		if pair.Inverse != nil {
			b.errorf(pending.span, "%s.%s is already the other side of %s.%s", pair.Entity.Raw, pair.Raw, pair.Inverse.Entity.Raw, pair.Inverse.Raw)
			continue
		}
		pair.Inverse, r.Inverse = r, pair
		if r.Display == nil {
			r.Display = defaultDisplay(r.Target)
		}
	}
}

// inferInverses adds the side nobody wrote to relations within a module: a
// ">" gets a "<" on its target named after the source's plural
// (Category.products), or after the source and the relation when that would
// be ambiguous (Person.people_by_manager); "-" and "<>" get their mirror.
// Name the other side yourself with "<" to choose its name.
func (b *builder) inferInverses() {
	count := map[[2]*Entity]int{}
	for _, e := range b.p.Entities {
		for _, r := range e.Relations {
			count[[2]*Entity{e, r.Target}]++
		}
	}
	for _, e := range b.p.Entities {
		declared := append([]*Relation(nil), e.Relations...)
		for _, r := range declared {
			if r.Inferred || r.Inverse != nil || r.Kind == OneToMany || (r.Entity.Module != nil && r.CrossesModules()) {
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
			if count[[2]*Entity{e, r.Target}] > 1 || e == r.Target {
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
		case *dsl.JobDecl:
			if j := b.jobs[d.Name.Name]; j != nil {
				check(j.Names, j.Pos, d.Name.Span)
			}
		case *dsl.ReportDecl:
			if r := b.reports[d.Name.Name]; r != nil {
				check(r.Names, r.Pos, d.Name.Span)
			}
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
	// A money field also stores its currency, in <field>_currency.
	for _, f := range e.Fields {
		if f.Type != TypeMoney {
			continue
		}
		col := f.Snake + "_currency"
		if prev, ok := seen[col]; ok {
			b.diags.Errorf(prev.pos, nameEnd(prev.pos, prev.raw), "field %s clashes with the currency column %s of money field %s", prev.raw, col, f.Raw).
				Suggestion = "remove it: the money field already stores its currency"
			continue
		}
		seen[col] = member{"currency of", f.Raw, f.Pos}
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
		src := r.Inverse
		clash := func(prev member) {
			d := b.diags.Errorf(src.Pos, nameEnd(src.Pos, src.Raw), "the other side of %s.%s would be %s.%s, which clashes with %s %s (%s)",
				src.Entity.Raw, src.Raw, e.Raw, r.Snake, prev.what, prev.raw, where(src.Pos, prev.pos))
			if r.Kind == ManyToOne {
				// Added for a "<": the user can declare it with another name.
				d.Suggestion = fmt.Sprintf("declare it on %s with another name, e.g. %s_ref > %s", e.Raw, r.Snake, src.Entity.Raw)
			} else {
				d.Suggestion = fmt.Sprintf("name it yourself on %s, e.g. %s_list %s %s", e.Raw, r.Snake, inverseSign(src.Kind), src.Entity.Raw)
			}
		}
		if prev, ok := seen[r.Snake]; ok {
			clash(prev)
			continue
		}
		seen[r.Snake] = member{"relation", r.Raw, r.Pos}
		if r.OwnsForeignKey() {
			fk := r.Snake + "_id"
			if prev, ok := seen[fk]; ok {
				clash(prev)
				continue
			}
			seen[fk] = member{"foreign key of", r.Raw, r.Pos}
		}
	}
}

// inverseSign is the sign that declares the other side of a relation of
// kind k.
func inverseSign(k RelationKind) string {
	switch k {
	case ManyToOne:
		return "<"
	case OneToMany:
		return ">"
	case OneToOne:
		return "-"
	}
	return "<>"
}

// ---- modules ----

var (
	moduleNameRE    = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	reservedModules = []string{"platform", "main", "cmd", "internal"}
	// layerNames describe a technical layer, not a part of the domain.
	layerNames = []string{
		"model", "models", "entity", "entities", "domain", "service", "services",
		"repository", "repositories", "repo", "repos", "handler", "handlers",
		"controller", "controllers", "dto", "dtos", "dao", "util", "utils",
		"helper", "helpers", "common", "shared", "misc", "core", "base", "lib",
	}
)

// buildModules reads each file's package declaration and assigns its
// entities and enums to that module. Every file with entities or enums must
// declare one.
func (b *builder) buildModules() {
	b.modules = map[string]*Module{}
	b.moduleOf = map[dsl.Decl]*Module{}
	for _, f := range b.files {
		var mod *Module
		reported := false
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *dsl.PackageDecl:
				mod = b.module(d)
			case *dsl.EntityDecl, *dsl.EnumDecl, *dsl.JobDecl, *dsl.ReportDecl:
				if mod != nil {
					b.moduleOf[d] = mod
					continue
				}
				if !reported {
					sp := d.SourceSpan()
					b.diags.Errorf(diag.Pos{File: f.Name, Line: 1, Col: 1}, diag.Pos{}, "%s has entities, enums or jobs but no package declaration", f.Name).
						Suggestion = fmt.Sprintf("start the file with the module its declarations belong to, e.g. package catalog (first declaration at line %d)", sp.Pos.Line)
					reported = true
				}
			}
		}
	}
	for _, m := range b.modules {
		b.p.Modules = append(b.p.Modules, m)
	}
	sort.Slice(b.p.Modules, func(i, j int) bool { return b.p.Modules[i].Raw < b.p.Modules[j].Raw })
}

// module returns the module a package declaration names, creating it and
// checking its name the first time.
func (b *builder) module(d *dsl.PackageDecl) *Module {
	name := d.Name.Name
	if m, ok := b.modules[name]; ok {
		return m
	}
	switch {
	case !moduleNameRE.MatchString(name):
		b.errorf(d.Name.Span, "module name %q must be lower case letters and digits, like a Go package", name).
			Suggestion = "e.g. package " + strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(asciiOnly(name)))
	case slices.Contains(reservedModules, name):
		b.errorf(d.Name.Span, "%s is reserved and cannot be a module name", name)
	case slices.Contains(layerNames, name):
		b.diags.Warnf(d.Name.Pos, d.Name.End, "module %s names a technical layer, not a part of the domain", name).
			Suggestion = "name the part of the domain it covers, e.g. catalog, orders, billing"
	}
	n, _ := makeNames(name)
	m := &Module{Names: n, Pos: d.Pos}
	b.modules[name] = m
	return m
}
