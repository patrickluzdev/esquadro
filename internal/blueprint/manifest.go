// Package blueprint reads blueprint manifests (blueprint.esq), loads
// blueprints from the binary or local folders, and validates a project's use
// of them: dependencies, options and annotations.
package blueprint

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// ManifestFile is the name of a blueprint's manifest inside its folder.
const ManifestFile = "blueprint.esq"

// ValueType is the type of an option or annotation key.
type ValueType string

const (
	TypeString ValueType = "string" // a quoted string
	TypeInt    ValueType = "int"    // a whole number
	TypeNumber ValueType = "number" // any number
	TypeBool   ValueType = "bool"   // true or false; a bare annotation key means true
	TypeName   ValueType = "name"   // an unquoted name
	TypeList   ValueType = "list"   // a [list] of values
	TypeField  ValueType = "field"  // the name of a field of the entity
	TypeFields ValueType = "fields" // a [list] of field names of the entity
)

var (
	optionTypes = []ValueType{TypeString, TypeInt, TypeNumber, TypeBool, TypeName, TypeList}
	keyTypes    = append(slices.Clone(optionTypes), TypeField, TypeFields)
)

// Target is where an annotation can be written.
type Target string

const (
	TargetApp      Target = "app"
	TargetEntity   Target = "entity"
	TargetField    Target = "field"
	TargetRelation Target = "relation"
	TargetJob      Target = "job"
)

var targets = []Target{TargetApp, TargetEntity, TargetField, TargetRelation, TargetJob}

// Manifest is a validated blueprint manifest.
type Manifest struct {
	Name    string
	Version string
	Doc     []string
	// Requires are the blueprints this one needs: an app without them is an
	// error.
	Requires []Dep
	// Optional are the blueprints this one works with when the app has
	// them: it comes after them, and its contributions to their slots are
	// dropped when they are missing.
	Optional []Dep
	Options  []*Spec
	// Annotations maps each target to the keys the blueprint accepts there.
	Annotations map[Target]*AnnotationSpec
	Slots       []*Slot
	GoRequires  []GoRequire
	Hooks       []*Hook
	// File is the manifest's name as shown in messages.
	File string
}

// Option returns the option with the given name, or nil.
func (m *Manifest) Option(name string) *Spec {
	for _, o := range m.Options {
		if o.Name == name {
			return o
		}
	}
	return nil
}

// Dep is a dependency on another blueprint.
type Dep struct {
	Name string
	Span dsl.Span
}

// Spec describes an option or an annotation key.
type Spec struct {
	Name     string
	Type     ValueType
	Required bool
	// Default is the typed default value, or nil.
	Default any
	Doc     []string
	Span    dsl.Span
}

// AnnotationSpec lists the keys of an annotation on one target.
type AnnotationSpec struct {
	Target Target
	Keys   []*Spec
	Doc    []string
}

// Key returns the key with the given name, or nil.
func (a *AnnotationSpec) Key(name string) *Spec {
	for _, k := range a.Keys {
		if k.Name == name {
			return k
		}
	}
	return nil
}

// Slot is an extension point opened by the blueprint's templates.
type Slot struct {
	Name string
	Doc  []string
	Span dsl.Span
}

// GoRequire is a Go module the generated project needs.
type GoRequire struct {
	Module  string
	Version string
	Span    dsl.Span
}

// Hook is a program run before or after generation.
type Hook struct {
	Event string // "pre-gen" or "post-gen"
	Path  string // relative to the blueprint folder
	// Writes are the globs, relative to the project root, of the files a
	// post-gen hook may write. They are tracked like generated files: an
	// edit by the user is a conflict.
	Writes []string
	// Edits are the globs of files a post-gen hook may change that stay the
	// user's, such as go.mod: never tracked, never a conflict.
	Edits []string
	Span  dsl.Span
}

var (
	blueprintNameRE = regexp.MustCompile(`^[a-z][a-z0-9]*([-_][a-z0-9]+)*$`)
	semverRE        = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	goVersionRE     = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+incompatible)?$`)
	slotNameRE      = regexp.MustCompile(`^[a-z0-9_-]+(\.[a-z0-9_-]+)+$`)
)

// ParseManifest parses and validates a blueprint manifest.
func ParseManifest(file string, src []byte) (*Manifest, diag.List) {
	f, diags := dsl.Parse(file, src)
	if diags.HasErrors() {
		return nil, diags
	}
	v := &validator{diags: diags}
	m := v.manifest(f)
	v.diags.Sort()
	if v.diags.HasErrors() {
		return nil, v.diags
	}
	return m, v.diags
}

type validator struct {
	diags diag.List
}

func (v *validator) errorf(s dsl.Span, format string, args ...any) *diag.Diagnostic {
	return v.diags.Errorf(s.Pos, s.End, format, args...)
}

func didYouMean(name string, cands []string) string {
	if s := diag.Suggest(name, cands); s != "" {
		return `did you mean "` + s + `"?`
	}
	return ""
}

func (v *validator) manifest(f *dsl.File) *Manifest {
	var decl *dsl.BlueprintDecl
	for _, d := range f.Decls {
		bd, ok := d.(*dsl.BlueprintDecl)
		switch {
		case !ok:
			v.errorf(d.SourceSpan(), "a blueprint manifest can only contain a blueprint block").
				Suggestion = "apps, entities and enums belong in the project's .esq files"
		case decl != nil:
			v.errorf(bd.Name.Span, "a manifest must have exactly one blueprint block; the first one is at line %d", decl.Pos.Line)
		default:
			decl = bd
		}
	}
	if decl == nil {
		v.diags.Errorf(diag.Pos{File: f.Name, Line: 1, Col: 1}, diag.Pos{}, "missing blueprint block").
			Suggestion = `start with: blueprint mine { version "0.1.0" }`
		return nil
	}

	m := &Manifest{Name: decl.Name.Name, Doc: decl.Doc, Annotations: map[Target]*AnnotationSpec{}, File: f.Name}
	if !blueprintNameRE.MatchString(m.Name) {
		v.errorf(decl.Name.Span, "blueprint name %q must be lower case letters and digits, separated by - or _", m.Name)
	}

	seen := map[string]dsl.Span{}
	once := func(key string, s dsl.Span) bool {
		if prev, ok := seen[key]; ok {
			v.errorf(s, "%s is already set at line %d", key, prev.Pos.Line)
			return false
		}
		seen[key] = s
		return true
	}
	deps := map[string]bool{}
	for _, it := range decl.Items {
		switch it := it.(type) {
		case *dsl.VersionItem:
			if !once("version", it.Span) {
				continue
			}
			if !semverRE.MatchString(it.Version.Text) {
				v.errorf(it.Version.Span, "version %q must look like 1.2.3", it.Version.Text)
			}
			m.Version = it.Version.Text
		case *dsl.DependsItem:
			for _, n := range it.Names {
				switch {
				case n.Name == m.Name:
					v.errorf(n.Span, "blueprint %s cannot depend on itself", m.Name)
				case deps[n.Name]:
					v.errorf(n.Span, "%s is already listed as a dependency", n.Name)
				default:
					deps[n.Name] = true
					d := Dep{Name: n.Name, Span: n.Span}
					if it.Keyword.Name == "requires" {
						m.Requires = append(m.Requires, d)
					} else {
						m.Optional = append(m.Optional, d)
					}
				}
			}
		case *dsl.OptionDecl:
			if !once("option "+it.Name.Name, it.Name.Span) {
				continue
			}
			if s := v.spec(it.Name, it.Type, it.Modifiers, it.Doc, optionTypes, "option"); s != nil {
				m.Options = append(m.Options, s)
			}
		case *dsl.AnnotationDecl:
			v.annotation(m, it, once)
		case *dsl.SlotItem:
			name := it.Name.Text
			if !once("slot "+strconv.Quote(name), it.Name.Span) {
				continue
			}
			if !slotNameRE.MatchString(name) || !strings.HasPrefix(name, m.Name+".") {
				v.errorf(it.Name.Span, "slot %q must start with the blueprint name, e.g. %q", name, m.Name+".routes")
				continue
			}
			m.Slots = append(m.Slots, &Slot{Name: name, Doc: it.Doc, Span: it.Span})
		case *dsl.GoRequireItem:
			mod := it.Module.Text
			if !once("go_require "+strconv.Quote(mod), it.Module.Span) {
				continue
			}
			if mod == "" || strings.ContainsAny(mod, " \t\\") || strings.HasPrefix(mod, "/") {
				v.errorf(it.Module.Span, "%q is not a valid Go module path", mod)
				continue
			}
			if !goVersionRE.MatchString(it.Version.Text) {
				v.errorf(it.Version.Span, "Go module version %q must look like v1.2.3", it.Version.Text)
				continue
			}
			m.GoRequires = append(m.GoRequires, GoRequire{Module: mod, Version: it.Version.Text, Span: it.Span})
		case *dsl.HookItem:
			if h := v.hook(it); h != nil {
				m.Hooks = append(m.Hooks, h)
			}
		}
	}
	if m.Version == "" {
		v.errorf(decl.Name.Span, "blueprint %s has no version", m.Name).Suggestion = `add: version "0.1.0"`
	}
	return m
}

func (v *validator) annotation(m *Manifest, d *dsl.AnnotationDecl, once func(string, dsl.Span) bool) {
	t := Target(d.Target.Name)
	if !slices.Contains(targets, t) {
		names := make([]string, len(targets))
		for i, x := range targets {
			names[i] = string(x)
		}
		e := v.errorf(d.Target.Span, "unknown annotation target %q", d.Target.Name)
		if s := didYouMean(d.Target.Name, names); s != "" {
			e.Suggestion = s
		} else {
			e.Suggestion = "targets are app, entity, field, relation and job"
		}
		return
	}
	if !once("annotation "+string(t), d.Target.Span) {
		return
	}
	types := keyTypes
	if t == TargetApp {
		types = optionTypes // there is no entity to take field names from
	}
	spec := &AnnotationSpec{Target: t, Doc: d.Doc}
	keys := map[string]bool{}
	for _, k := range d.Keys {
		if keys[k.Name.Name] {
			v.errorf(k.Name.Span, "key %s is already declared", k.Name.Name)
			continue
		}
		keys[k.Name.Name] = true
		if (t == TargetApp || t == TargetJob) && (k.Type.Name == string(TypeField) || k.Type.Name == string(TypeFields)) {
			v.errorf(k.Type.Span, "%s is not available on %s annotations", k.Type.Name, t).
				Suggestion = "field and fields name fields of an entity; use them on entity, field or relation annotations"
			continue
		}
		if s := v.spec(k.Name, k.Type, k.Modifiers, k.Doc, types, "key"); s != nil {
			spec.Keys = append(spec.Keys, s)
		}
	}
	m.Annotations[t] = spec
}

// spec validates an option or key declaration: its type and the required
// and default modifiers.
func (v *validator) spec(name, typ dsl.Ident, mods []*dsl.Modifier, doc []string, types []ValueType, what string) *Spec {
	s := &Spec{Name: name.Name, Type: ValueType(typ.Name), Doc: doc, Span: name.Span}
	if !slices.Contains(types, s.Type) {
		names := make([]string, len(types))
		for i, t := range types {
			names[i] = string(t)
		}
		e := v.errorf(typ.Span, "unknown %s type %q", what, typ.Name)
		if sug := didYouMean(typ.Name, names); sug != "" {
			e.Suggestion = sug
		} else {
			e.Suggestion = "types are " + strings.Join(names, ", ")
		}
		return nil
	}
	ok := true
	var def *dsl.Modifier
	for _, m := range mods {
		switch m.Name.Name {
		case "required":
			if m.HasParens {
				v.errorf(m.Span, "required takes no arguments")
				ok = false
			}
			s.Required = true
		case "default":
			if !m.HasParens || len(m.Args) != 1 {
				v.errorf(m.Span, "default needs exactly one value")
				ok = false
				continue
			}
			def = m
		default:
			v.errorf(m.Name.Span, "unknown modifier %q", m.Name.Name).Suggestion = didYouMean(m.Name.Name, []string{"required", "default"})
			ok = false
		}
	}
	if def != nil {
		if s.Required {
			v.errorf(def.Span, "%s %s cannot be both required and have a default", map[string]string{"option": "an", "key": "a"}[what], what)
			return nil
		}
		val, err := Convert(def.Args[0], s.Type)
		if err != "" {
			v.errorf(def.Args[0].Span, "default of %s: %s", s.Name, err)
			return nil
		}
		s.Default = val
	}
	if !ok {
		return nil
	}
	return s
}

func (v *validator) hook(h *dsl.HookItem) *Hook {
	ev := h.Event.Name
	if ev != "pre-gen" && ev != "post-gen" {
		v.errorf(h.Event.Span, "unknown hook event %q", ev).Suggestion = didYouMean(ev, []string{"pre-gen", "post-gen"})
		return nil
	}
	p := h.Path.Text
	if !isRelativePath(p) {
		v.errorf(h.Path.Span, "hook path %q must be relative to the blueprint folder and stay inside it", p)
		return nil
	}
	out := &Hook{Event: ev, Path: path.Clean(p), Span: h.Span}
	var ok bool
	if out.Writes, ok = v.globs(h.Writes, "writes", ev); !ok {
		return nil
	}
	if out.Edits, ok = v.globs(h.Edits, "edits", ev); !ok {
		return nil
	}
	return out
}

// globs validates the patterns of a hook's writes or edits.
func (v *validator) globs(val *dsl.Value, what, event string) ([]string, bool) {
	if val == nil {
		return nil, true
	}
	if event != "post-gen" {
		v.errorf(val.Span, "%s is only allowed on post-gen hooks", what).
			Suggestion = "pre-gen hooks return files to the engine instead of writing them"
		return nil, false
	}
	vals := []dsl.Value{*val}
	if val.Kind == dsl.ListValue {
		vals = val.List
	}
	var out []string
	for _, w := range vals {
		if w.Kind != dsl.StringValue {
			v.errorf(w.Span, "%s must be a string or a list of strings", what)
			return nil, false
		}
		if !isRelativePath(w.Text) {
			v.errorf(w.Span, "%q must be a path relative to the project and stay inside it", w.Text)
			return nil, false
		}
		if _, err := path.Match(w.Text, ""); err != nil {
			v.errorf(w.Span, "invalid pattern %q", w.Text)
			return nil, false
		}
		out = append(out, w.Text)
	}
	return out, true
}

// isRelativePath reports whether p is a non-empty relative slash path that
// does not climb out of its root.
func isRelativePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	c := path.Clean(p)
	return c != ".." && !strings.HasPrefix(c, "../")
}

// Convert turns a literal into the Go value for a type: string, int64,
// float64, bool, []string (fields) or []any (list). It returns a message
// when the value does not fit the type.
func Convert(v dsl.Value, t ValueType) (any, string) {
	switch t {
	case TypeString:
		if v.Kind == dsl.StringValue {
			return v.Text, ""
		}
		return nil, "expected a quoted string"
	case TypeInt:
		if v.Kind == dsl.NumberValue && !strings.Contains(v.Text, ".") {
			n, err := strconv.ParseInt(v.Text, 10, 64)
			if err == nil {
				return n, ""
			}
		}
		return nil, "expected a whole number"
	case TypeNumber:
		if v.Kind == dsl.NumberValue {
			n, err := strconv.ParseFloat(v.Text, 64)
			if err == nil {
				return n, ""
			}
		}
		return nil, "expected a number"
	case TypeBool:
		if v.Kind == dsl.BoolValue {
			return v.Text == "true", ""
		}
		return nil, "expected true or false"
	case TypeName, TypeField:
		if v.Kind == dsl.IdentValue {
			return v.Text, ""
		}
		return nil, "expected a name (without quotes)"
	case TypeFields:
		if v.Kind == dsl.IdentValue {
			return []string{v.Text}, ""
		}
		if v.Kind == dsl.ListValue {
			out := make([]string, 0, len(v.List))
			for _, e := range v.List {
				if e.Kind != dsl.IdentValue {
					return nil, "expected a list of field names, e.g. [name, status]"
				}
				out = append(out, e.Text)
			}
			return out, ""
		}
		return nil, "expected a list of field names, e.g. [name, status]"
	case TypeList:
		if v.Kind != dsl.ListValue {
			return nil, "expected a [list]"
		}
		out := make([]any, 0, len(v.List))
		for _, e := range v.List {
			out = append(out, scalar(e))
		}
		return out, ""
	}
	return nil, fmt.Sprintf("unknown type %q", t)
}

// scalar converts a list element without a declared type.
func scalar(v dsl.Value) any {
	switch v.Kind {
	case dsl.NumberValue:
		if n, err := strconv.ParseInt(v.Text, 10, 64); err == nil {
			return n
		}
		n, _ := strconv.ParseFloat(v.Text, 64)
		return n
	case dsl.BoolValue:
		return v.Text == "true"
	case dsl.ListValue:
		out := make([]any, 0, len(v.List))
		for _, e := range v.List {
			out = append(out, scalar(e))
		}
		return out
	}
	return v.Text
}
