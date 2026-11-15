// Package compose renders the templates of a project's blueprints into an
// in-memory set of files. Blueprints are applied as layers in generation
// order (a later blueprint replaces a file an earlier one produced) and
// extend each other's files through named slots.
//
// Blueprint layout:
//
//	templates/          mirrors the generated project
//	  README.md.tmpl                         rendered once
//	  internal/{{.Entity.Snake}}_gen.go.tmpl rendered once per entity
//	  internal/{{.Enum.Snake}}_gen.go.tmpl   rendered once per enum
//	  {{.Relation.Snake}}.go.tmpl            once per relation, each side
//	  notes.md.once.tmpl                     owned by the user after creation
//	  static/logo.png                        copied as is (no .tmpl)
//	slots/
//	  platform.routes.tmpl      contribution to the slot platform.routes
//	  platform.routes/*.tmpl    several contributions to the same slot
//
// A template opens a slot with {{slot "platform.routes" .}}; contributions are
// rendered at that point with the caller's data. A template whose output is
// only whitespace produces no file. Generated .go files are gofmt'ed.
package compose

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

const (
	templatesDir = "templates"
	slotsDir     = "slots"
	tmplExt      = ".tmpl"
	onceExt      = ".once.tmpl"
)

// File is a generated file.
type File struct {
	Path    string
	Content []byte
	// Blueprint is the name of the blueprint that produced the file and
	// Template the template's path inside it.
	Blueprint string
	Template  string
	// Once marks files owned by the user: created if missing, never updated.
	Once bool
	// Replaced lists, in order, the blueprints whose file at this path was
	// replaced by a later layer.
	Replaced []string
	// Slots lists the slots opened while rendering the file and who
	// contributed to them.
	Slots []SlotUse
}

// SlotUse records the contributions to one slot in one file.
type SlotUse struct {
	Slot         string
	Contributors []string
}

// Result is the set of generated files, sorted by path.
type Result struct {
	Files []*File
}

// File returns the generated file at path, or nil.
func (r *Result) File(p string) *File {
	for _, f := range r.Files {
		if f.Path == p {
			return f
		}
	}
	return nil
}

// Data is what templates see as ".".
type Data struct {
	App      *project.App
	Entities []*project.Entity
	Enums    []*project.Enum
	Modules  []*project.Module
	// Entity is set in templates rendered per entity (and per relation),
	// Enum per enum and Module per module; per entity and per enum, Module is theirs. Slot
	// contributions see their caller's values.
	Entity *project.Entity
	Enum   *project.Enum
	Module *project.Module
	// Relation is set in templates rendered per relation: each side of
	// every relation, with Entity and Module those of its side.
	Relation *project.Relation
	// Options are the options of the blueprint whose template is running.
	Options map[string]any
	// Blueprint is the name of the blueprint whose template is running.
	Blueprint string
}

// Compose renders every blueprint of the set. Diagnostics point at template
// files; Sources returns their contents for rendering.
func Compose(p *project.Project, set *blueprint.Set) (*Result, *Sources, diag.List) {
	return ComposeWith(p, set, nil)
}

// ComposeWith is Compose with extra template functions per blueprint name:
// built-in blueprints give their templates helpers written in Go.
func ComposeWith(p *project.Project, set *blueprint.Set, extra map[string]template.FuncMap) (*Result, *Sources, diag.List) {
	c := &composer{p: p, set: set, src: &Sources{files: map[string][]byte{}}, extra: extra}
	res := c.compose()
	c.diags.Sort()
	return res, c.src, c.diags
}

// Sources holds the contents of the template files read during composition.
type Sources struct {
	files map[string][]byte
}

// Get returns a template file's contents, or nil.
func (s *Sources) Get(name string) []byte { return s.files[name] }

type composer struct {
	p     *project.Project
	set   *blueprint.Set
	src   *Sources
	diags diag.List

	// declared maps slot name to the blueprint that declared it.
	declared map[string]string
	// contribs maps slot name to its contributions, in generation order.
	contribs map[string][]*contribution

	// extra are template functions by blueprint name.
	extra map[string]template.FuncMap

	// Rendering state.
	cur   *File    // file being rendered
	stack []string // slots being rendered, for cycle detection
}

type contribution struct {
	bp   *blueprint.Blueprint
	tmpl *template.Template
}

// display is the name of a blueprint file in messages: the blueprint's
// location joined with the file's path inside it.
func display(bp *blueprint.Blueprint, name string) string {
	if bp.Location == "built-in" {
		return "builtin/" + bp.Manifest.Name + "/" + name
	}
	return path.Join(strings.ReplaceAll(bp.Location, "\\", "/"), name)
}

func (c *composer) errorf(file string, line int, format string, args ...any) {
	pos := diag.Pos{File: file}
	if line > 0 {
		pos.Line, pos.Col = line, 1
	}
	c.diags.Errorf(pos, diag.Pos{}, format, args...)
}

// templateErr turns a text/template error into a diagnostic at the line it
// names, when it names one.
var templateErrRE = regexp.MustCompile(`^template: ([^:]+):(\d+)(?::(\d+))?: (.*)$`)

func (c *composer) templateError(file string, err error) {
	// An error raised inside a slot is reported where it really happened:
	// in the contribution, or at the caller for slot errors themselves.
	if se, ok := errors.AsType[*slotError](err); ok {
		err = se.err
	}
	msg := err.Error()
	if m := templateErrRE.FindStringSubmatch(msg); m != nil {
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		pos := diag.Pos{File: m[1], Line: line, Col: max(col, 1)}
		d := c.diags.Errorf(pos, diag.Pos{}, "%s", cleanTemplateMsg(m[4]))
		for _, v := range []string{"Entity", "Enum", "Relation"} {
			if strings.Contains(m[4], "nil pointer evaluating *project."+v+".") {
				d.Suggestion = fmt.Sprintf(".%s is only set in templates whose file name uses .%s (rendered once per %s) and in slots opened from them",
					v, v, strings.ToLower(v))
			}
		}
		return
	}
	c.errorf(file, 0, "%s", msg)
}

// slotError carries an error from inside a slot up through the templates
// that opened it, so only the original error is reported.
type slotError struct{ err error }

func (e *slotError) Error() string { return e.err.Error() }
func (e *slotError) Unwrap() error { return e.err }

// cleanTemplateMsg drops the "executing <name> at <...>:" prefix that
// text/template adds, keeping the useful part.
func cleanTemplateMsg(s string) string {
	if _, rest, ok := strings.Cut(s, ">: "); ok && strings.HasPrefix(s, "executing ") {
		return rest
	}
	return s
}

func (c *composer) compose() *Result {
	c.declared = map[string]string{}
	for _, bp := range c.set.Blueprints {
		for _, s := range bp.Manifest.Slots {
			c.declared[s.Name] = bp.Manifest.Name
		}
	}
	c.contribs = map[string][]*contribution{}
	for _, bp := range c.set.Blueprints {
		c.loadContributions(bp)
	}

	files := map[string]*File{}
	for _, bp := range c.set.Blueprints {
		own := map[string]string{} // path -> template, within this blueprint
		for _, f := range c.renderBlueprint(bp) {
			if prev, ok := own[f.Path]; ok {
				c.errorf(display(bp, templatesDir+"/"+f.Template), 0, "%s and %s both generate %s", prev, f.Template, f.Path)
				continue
			}
			own[f.Path] = f.Template
			if prev, ok := files[f.Path]; ok {
				f.Replaced = append(slices.Clone(prev.Replaced), prev.Blueprint)
			}
			files[f.Path] = f
		}
	}

	res := &Result{}
	for _, f := range files {
		res.Files = append(res.Files, f)
	}
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res
}

// ---- slot contributions ----

func (c *composer) loadContributions(bp *blueprint.Blueprint) {
	err := fs.WalkDir(bp.FS, slotsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, tmplExt) {
			return err
		}
		rel := strings.TrimPrefix(p, slotsDir+"/")
		// slots/<slot>.tmpl or slots/<slot>/<anything>.tmpl
		slot := strings.TrimSuffix(rel, tmplExt)
		if dir, _, ok := strings.Cut(rel, "/"); ok {
			slot = dir
		}
		name := display(bp, p)
		if _, ok := c.declared[slot]; !ok && c.optionalAbsent(bp, slot) {
			// A slot of an optional blueprint the app does not use: the
			// contribution has nowhere to go, and that is fine.
			return nil
		}
		if _, ok := c.declared[slot]; !ok {
			var names []string
			for n := range c.declared {
				names = append(names, n)
			}
			sort.Strings(names)
			msg := fmt.Sprintf("contribution to slot %q, which no blueprint of this app declares", slot)
			sug := diag.Suggest(slot, names)
			owner, _, _ := strings.Cut(slot, ".")
			switch {
			case sug != "":
				msg += fmt.Sprintf(` (did you mean "%s"?)`, sug)
			case !slices.ContainsFunc(c.set.Blueprints, func(b *blueprint.Blueprint) bool { return b.Manifest.Name == owner }):
				msg += fmt.Sprintf("; if it is for blueprint %s when the app uses it, list it in %s's manifest: optional %s", owner, bp.Manifest.Name, owner)
			}
			c.errorf(name, 0, "%s", msg)
			return nil
		}
		t := c.parse(bp, name, p)
		if t != nil {
			c.contribs[slot] = append(c.contribs[slot], &contribution{bp: bp, tmpl: t})
		}
		return nil
	})
	if err != nil && !isNotExist(err) {
		c.errorf(display(bp, slotsDir), 0, "cannot read slots: %v", err)
	}
}

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// parse reads and parses a template file of a blueprint.
func (c *composer) parse(bp *blueprint.Blueprint, name, p string) *template.Template {
	src, err := fs.ReadFile(bp.FS, p)
	if err != nil {
		c.errorf(name, 0, "cannot read template: %v", err)
		return nil
	}
	c.src.files[name] = src
	t, err := template.New(name).Option("missingkey=error").Funcs(c.funcs(bp)).Parse(string(src))
	if err != nil {
		c.templateError(name, err)
		return nil
	}
	return t
}

// ---- template functions ----

func (c *composer) funcs(bp *blueprint.Blueprint) template.FuncMap {
	self := bp.Manifest.Name
	funcs := c.baseFuncs(self)
	maps.Copy(funcs, c.extra[self])
	return funcs
}

func (c *composer) baseFuncs(self string) template.FuncMap {
	return template.FuncMap{
		"slot":      c.slot,
		"slotParts": c.slotParts,
		// optionsOf returns the options of another blueprint of the app,
		// nil when the app does not use it: a blueprint that works on
		// another one's data reads its settings.
		"optionsOf": func(name string) map[string]any { return c.set.Options[name] },
		// uses reports whether the app uses a blueprint, for what a
		// template writes only alongside it.
		"uses": func(name string) bool {
			return slices.ContainsFunc(c.set.Blueprints, func(b *blueprint.Blueprint) bool { return b.Manifest.Name == name })
		},
		// ann returns this blueprint's annotation values on a node (app,
		// entity, field or relation), or nil when it has none.
		"ann": func(node any) map[string]any { return annOf(node)[self] },
		// annOf returns another blueprint's annotation values on a node.
		"annOf":     func(name string, node any) map[string]any { return annOf(node)[name] },
		"join":      func(sep string, items []string) string { return strings.Join(items, sep) },
		"contains":  strings.Contains,
		"strs":      strs,
		"goStrings": goStrings,
		"goType":    goType,
		"goImports": goImports,
		"quote":     strconv.Quote,
		// rawQuote writes s as a Go raw string when it can, which regexps
		// read best in.
		"rawQuote": func(s string) string {
			if strings.ContainsAny(s, "`\r") {
				return strconv.Quote(s)
			}
			return "`" + s + "`"
		},
		"importBlock": importBlock,
		"goDoc":       goDoc,
		"enumValues":  enumValues,
		"lower":       strings.ToLower,
		"upper":       strings.ToUpper,
		// indent prefixes every non-empty line of s with n spaces.
		"indent": func(n int, s string) string {
			pad := strings.Repeat(" ", n)
			lines := strings.Split(s, "\n")
			for i, l := range lines {
				if l != "" {
					lines[i] = pad + l
				}
			}
			return strings.Join(lines, "\n")
		},
	}
}

func annOf(node any) project.Ann {
	switch n := node.(type) {
	case *project.App:
		return n.Ann
	case *project.Entity:
		return n.Ann
	case *project.Field:
		return n.Ann
	case *project.Relation:
		return n.Ann
	}
	return nil
}

// slot renders every contribution to a slot with the caller's data and the
// contributing blueprint's options.
func (c *composer) slot(name string, data Data) (string, error) {
	parts, err := c.slotParts(name, data)
	return strings.Join(parts, "\n"), err
}

// slotParts renders every contribution to a slot, each without the blank
// lines around it, for a template that joins them its own way (e.g. with a
// blank line between YAML entries).
func (c *composer) slotParts(name string, data Data) ([]string, error) {
	if _, ok := c.declared[name]; !ok {
		return nil, fmt.Errorf("slot %q is not declared by any blueprint of this app", name)
	}
	if slices.Contains(c.stack, name) {
		return nil, &slotError{fmt.Errorf("slots open each other in a cycle: %s -> %s", strings.Join(c.stack, " -> "), name)}
	}
	c.stack = append(c.stack, name)
	defer func() { c.stack = c.stack[:len(c.stack)-1] }()

	use := SlotUse{Slot: name}
	var parts []string
	for _, ct := range c.contribs[name] {
		d := data
		d.Options = c.set.Options[ct.bp.Manifest.Name]
		d.Blueprint = ct.bp.Manifest.Name
		var buf bytes.Buffer
		if err := ct.tmpl.Execute(&buf, d); err != nil {
			if inner, ok := errors.AsType[*slotError](err); ok {
				return nil, inner
			}
			return nil, &slotError{err}
		}
		out := strings.Trim(buf.String(), "\n")
		if strings.TrimSpace(out) == "" {
			continue
		}
		parts = append(parts, out)
		if !slices.Contains(use.Contributors, ct.bp.Manifest.Name) {
			use.Contributors = append(use.Contributors, ct.bp.Manifest.Name)
		}
	}
	if c.cur != nil && len(use.Contributors) > 0 {
		c.cur.Slots = append(c.cur.Slots, use)
	}
	return parts, nil
}

// optionalAbsent reports whether slot belongs, by its prefix, to a blueprint
// bp lists as optional and the app does not use.
func (c *composer) optionalAbsent(bp *blueprint.Blueprint, slot string) bool {
	owner, _, _ := strings.Cut(slot, ".")
	if !slices.ContainsFunc(bp.Manifest.Optional, func(d blueprint.Dep) bool { return d.Name == owner }) {
		return false
	}
	return !slices.ContainsFunc(c.set.Blueprints, func(b *blueprint.Blueprint) bool { return b.Manifest.Name == owner })
}

// ---- templates ----

var (
	relationRef = regexp.MustCompile(`\.Relation\b`)
	entityRef   = regexp.MustCompile(`\.Entity\b`)
	enumRef     = regexp.MustCompile(`\.Enum\b`)
	moduleRef   = regexp.MustCompile(`\.Module\b`)
)

func (c *composer) renderBlueprint(bp *blueprint.Blueprint) []*File {
	var out []*File
	err := fs.WalkDir(bp.FS, templatesDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out = append(out, c.renderTemplate(bp, p)...)
		return nil
	})
	if err != nil && !isNotExist(err) {
		c.errorf(display(bp, templatesDir), 0, "cannot read templates: %v", err)
	}
	return out
}

func (c *composer) renderTemplate(bp *blueprint.Blueprint, p string) []*File {
	rel := strings.TrimPrefix(p, templatesDir+"/")
	name := display(bp, p)
	once := strings.HasSuffix(rel, onceExt)
	isTmpl := strings.HasSuffix(rel, tmplExt)
	outPath := strings.TrimSuffix(strings.TrimSuffix(rel, onceExt), tmplExt)

	perRelation := relationRef.MatchString(outPath)
	// .Relation.Entity is per relation; .Entity.Module and .Enum.Module are
	// per entity or enum, not per module.
	perEntity := !perRelation && entityRef.MatchString(outPath)
	perEnum := enumRef.MatchString(outPath)
	perModule := !perRelation && !perEntity && !perEnum && moduleRef.MatchString(outPath)
	if (perEntity || perRelation) && perEnum {
		c.errorf(name, 0, "a template path cannot use both .Enum and .Entity or .Relation")
		return nil
	}
	pathTmpl, err := template.New(name).Option("missingkey=error").Funcs(c.funcs(bp)).Parse(outPath)
	if err != nil {
		c.errorf(name, 0, "invalid template in file name: %v", err)
		return nil
	}

	var body *template.Template
	var raw []byte
	if isTmpl {
		if body = c.parse(bp, name, p); body == nil {
			return nil
		}
	} else if raw, err = fs.ReadFile(bp.FS, p); err != nil {
		c.errorf(name, 0, "cannot read file: %v", err)
		return nil
	}

	whole := Data{
		App: c.p.App, Entities: c.p.Entities, Enums: c.p.Enums, Modules: c.p.Modules,
		Options: c.set.Options[bp.Manifest.Name], Blueprint: bp.Manifest.Name,
	}
	var datas []Data
	switch {
	case perRelation:
		for _, e := range c.p.Entities {
			for _, r := range e.Relations {
				d := whole
				d.Relation, d.Entity, d.Module = r, e, e.Module
				datas = append(datas, d)
			}
		}
	case perEntity:
		for _, e := range c.p.Entities {
			d := whole
			d.Entity, d.Module = e, e.Module
			datas = append(datas, d)
		}
	case perEnum:
		for _, e := range c.p.Enums {
			d := whole
			d.Enum, d.Module = e, e.Module
			datas = append(datas, d)
		}
	case perModule:
		for _, m := range c.p.Modules {
			d := whole
			d.Module = m
			datas = append(datas, d)
		}
	default:
		datas = []Data{whole}
	}

	var out []*File
	for _, d := range datas {
		var pb bytes.Buffer
		if err := pathTmpl.Execute(&pb, d); err != nil {
			c.errorf(name, 0, "invalid template in file name: %s", cleanTemplateMsg(strings.TrimPrefix(err.Error(), "template: "+name+": ")))
			return out
		}
		target, ok := CleanTarget(pb.String())
		if !ok {
			c.errorf(name, 0, "file name renders to %q, which is not a path inside the project", pb.String())
			return out
		}
		f := &File{Path: target, Blueprint: bp.Manifest.Name, Template: rel, Once: once}
		if body == nil {
			f.Content = raw
			out = append(out, f)
			continue
		}
		c.cur = f
		var buf bytes.Buffer
		err := body.Execute(&buf, d)
		c.cur = nil
		if err != nil {
			c.templateError(name, err)
			return out
		}
		if strings.TrimSpace(buf.String()) == "" {
			continue // conditional template: nothing to generate
		}
		content := buf.Bytes()
		if strings.HasSuffix(target, ".go") {
			formatted, err := format.Source(content)
			if err != nil {
				c.goError(name, target, content, err)
				return out
			}
			content = formatted
		}
		f.Content = content
		out = append(out, f)
	}
	return out
}

// CleanTarget validates a generated file path: relative, inside the
// project, not in esquadro/ (esquadro's own folder) and without empty
// segments.
func CleanTarget(p string) (string, bool) {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.Contains(p, "//") || strings.HasSuffix(p, "/") {
		return "", false
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || c == "esquadro" || strings.HasPrefix(c, "esquadro/") {
		return "", false
	}
	return c, true
}

var goErrRE = regexp.MustCompile(`^(\d+):(\d+): `)

// goError reports generated Go code that does not parse, quoting the
// offending generated line: template authors never see the output.
func (c *composer) goError(name, target string, content []byte, err error) {
	d := c.diags.Errorf(diag.Pos{File: name}, diag.Pos{}, "generated %s is not valid Go: %v", target, err)
	if m := goErrRE.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		lines := strings.Split(string(content), "\n")
		if n >= 1 && n <= len(lines) {
			d.Suggestion = fmt.Sprintf("line %d of the generated file is: %s", n, strings.TrimSpace(lines[n-1]))
		}
	}
}
