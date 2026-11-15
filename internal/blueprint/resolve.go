package blueprint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Set is the resolved set of blueprints of a project.
type Set struct {
	// Blueprints are in generation order: every blueprint comes after the
	// ones it requires or runs after; ties keep the order of the app block.
	Blueprints []*Blueprint
	// Options holds each blueprint's options with defaults filled in, by
	// blueprint name then option name.
	Options map[string]map[string]any
}

// Get returns the blueprint with the given name, or nil.
func (s *Set) Get(name string) *Blueprint {
	for _, b := range s.Blueprints {
		if b.Manifest.Name == name {
			return b
		}
	}
	return nil
}

// Resolve loads the blueprints a project uses and validates the project
// against them: dependencies, options and annotations. Validated annotation
// values are stored in the model's Ann fields.
func Resolve(p *project.Project, l *Loader) (*Set, diag.List) {
	r := &resolver{p: p, l: l, set: &Set{Options: map[string]map[string]any{}}}
	r.resolve()
	r.diags.Sort()
	return r.set, r.diags
}

type resolver struct {
	p     *project.Project
	l     *Loader
	set   *Set
	diags diag.List
	// loaded keeps app order; byName indexes it.
	loaded []*Blueprint
	byName map[string]*Blueprint
	// failed holds names of blueprints that are used but failed to load,
	// so their annotations are not reported as unknown.
	failed map[string]bool
}

func (r *resolver) errorf(s dsl.Span, format string, args ...any) *diag.Diagnostic {
	return r.diags.Errorf(s.Pos, s.End, format, args...)
}

func (r *resolver) resolve() {
	r.byName = map[string]*Blueprint{}
	r.failed = map[string]bool{}
	r.l.unfetched = 0
	for _, ref := range r.p.App.Blueprints {
		b, diags := r.l.Load(ref)
		r.diags = append(r.diags, diags...)
		if b == nil {
			if ref.Source == project.SourceBuiltin {
				r.failed[ref.Ref] = true
			}
			continue
		}
		name := b.Manifest.Name
		if prev, ok := r.byName[name]; ok {
			r.errorf(ref.RefSpan, "blueprint %s (from %s) has the same name as the one at line %d", name, b.Location, prev.Ref.Pos.Line)
			continue
		}
		r.byName[name] = b
		r.loaded = append(r.loaded, b)
	}

	for _, b := range r.loaded {
		for _, d := range b.Manifest.Requires {
			if r.byName[d.Name] == nil && !r.failed[d.Name] && r.l.unfetched == 0 {
				r.errorf(b.Ref.RefSpan, "blueprint %s requires %s", b.Manifest.Name, d.Name).
					Suggestion = "add to the app: blueprint " + d.Name
			}
		}
		r.set.Options[b.Manifest.Name] = r.options(b)
		b.Ref.Name, b.Ref.Values = b.Manifest.Name, r.set.Options[b.Manifest.Name]
	}
	r.set.Blueprints = r.order()
	r.annotations()
}

// options validates the options written for a blueprint in the app block and
// fills in defaults.
func (r *resolver) options(b *Blueprint) map[string]any {
	m := b.Manifest
	out := map[string]any{}
	for _, o := range b.Ref.Options {
		spec := m.Option(o.Key.Name)
		if spec == nil {
			names := make([]string, len(m.Options))
			for i, s := range m.Options {
				names[i] = s.Name
			}
			e := r.errorf(o.Key.Span, "blueprint %s has no option %q", m.Name, o.Key.Name)
			if s := didYouMean(o.Key.Name, names); s != "" {
				e.Suggestion = s
			} else if len(names) > 0 {
				e.Suggestion = "options are " + joinNames(names)
			} else {
				e.Suggestion = "this blueprint has no options"
			}
			continue
		}
		v, msg := Convert(o.Value, spec.Type)
		if msg != "" {
			r.errorf(o.Value.Span, "option %s of blueprint %s: %s", o.Key.Name, m.Name, msg)
			continue
		}
		out[spec.Name] = v
	}
	for _, s := range m.Options {
		if _, ok := out[s.Name]; ok {
			continue
		}
		switch {
		case s.Default != nil:
			out[s.Name] = s.Default
		case s.Required:
			r.errorf(b.Ref.RefSpan, "option %s of blueprint %s is required", s.Name, m.Name).
				Suggestion = fmt.Sprintf("add it: blueprint %s { %s %s }", refString(b.Ref), s.Name, Example(s.Type))
		}
	}
	return out
}

// order sorts the blueprints so that each one comes after those it requires
// or runs after (when present), keeping app order otherwise. Cycles are
// reported and broken in app order.
func (r *resolver) order() []*Blueprint {
	deps := map[*Blueprint][]*Blueprint{}
	for _, b := range r.loaded {
		for _, d := range slices.Concat(b.Manifest.Requires, b.Manifest.Optional) {
			if dep := r.byName[d.Name]; dep != nil {
				deps[b] = append(deps[b], dep)
			}
		}
	}
	var out []*Blueprint
	done := map[*Blueprint]bool{}
	for len(out) < len(r.loaded) {
		progress := false
		for _, b := range r.loaded {
			if done[b] {
				continue
			}
			ready := true
			for _, d := range deps[b] {
				ready = ready && done[d]
			}
			if ready {
				out = append(out, b)
				done[b] = true
				progress = true
				break // restart so earlier app entries win ties
			}
		}
		if !progress {
			// Every remaining blueprint waits on another: report the cycle
			// starting from the first one in app order and break it there.
			var start *Blueprint
			for _, b := range r.loaded {
				if !done[b] {
					start = b
					break
				}
			}
			r.reportCycle(start, deps, done)
			out = append(out, start)
			done[start] = true
		}
	}
	return out
}

func (r *resolver) reportCycle(start *Blueprint, deps map[*Blueprint][]*Blueprint, done map[*Blueprint]bool) {
	path := []*Blueprint{start}
	seen := map[*Blueprint]int{start: 0}
	cur := start
	for {
		var next *Blueprint
		for _, d := range deps[cur] {
			if !done[d] {
				next = d
				break
			}
		}
		if next == nil {
			return
		}
		if i, ok := seen[next]; ok {
			names := make([]string, 0, len(path)-i+1)
			for _, b := range path[i:] {
				names = append(names, b.Manifest.Name)
			}
			names = append(names, next.Manifest.Name)
			r.errorf(next.Ref.RefSpan, "blueprints depend on each other in a cycle: %s", strings.Join(names, " -> "))
			return
		}
		seen[next] = len(path)
		path = append(path, next)
		cur = next
	}
}

// ---- annotations ----

// annotations validates every annotation in the project and stores the
// resolved values in the model.
func (r *resolver) annotations() {
	app := r.p.App
	app.Ann = r.annotate(app.Annotations, TargetApp, nil)
	for _, e := range r.p.Entities {
		e.Ann = r.annotate(e.Annotations, TargetEntity, e)
		for _, f := range e.Fields {
			f.Ann = r.annotate(f.Annotations, TargetField, e)
		}
		for _, rel := range e.Relations {
			if !rel.Inferred {
				rel.Ann = r.annotate(rel.Annotations, TargetRelation, e)
			}
		}
	}
}

func (r *resolver) annotate(anns []*dsl.Annotation, t Target, e *project.Entity) project.Ann {
	if len(anns) == 0 {
		return nil
	}
	out := project.Ann{}
	seen := map[string]*dsl.Annotation{}
	for _, a := range anns {
		name := a.Name.Name
		if prev, ok := seen[name]; ok {
			r.errorf(a.Span, "@%s is already used here at column %d", name, prev.Pos.Col).
				Suggestion = "put all keys in one annotation"
			continue
		}
		seen[name] = a
		b := r.byName[name]
		if b == nil {
			// With a blueprint from git not fetched yet, the annotation may
			// be its own.
			if !r.failed[name] && r.l.unfetched == 0 {
				r.unknownAnnotation(a)
			}
			continue
		}
		spec := b.Manifest.Annotations[t]
		if spec == nil {
			var where []string
			for _, x := range targets {
				if b.Manifest.Annotations[x] != nil {
					where = append(where, plural(x))
				}
			}
			e := r.errorf(a.Span, "blueprint %s has no annotation for %s", name, plural(t))
			if len(where) > 0 {
				e.Suggestion = "@" + name + " can be used on " + joinNames(where)
			}
			continue
		}
		if vals := r.annotationArgs(a, spec, name, e); vals != nil {
			out[name] = vals
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *resolver) unknownAnnotation(a *dsl.Annotation) {
	name := a.Name.Name
	if slices.Contains(r.l.BuiltinNames(), name) {
		r.errorf(a.Span, "@%s needs blueprint %s, which this app does not use", name, name).
			Suggestion = "add to the app: blueprint " + name
		return
	}
	cands := r.l.BuiltinNames()
	for _, b := range r.loaded {
		cands = append(cands, b.Manifest.Name)
	}
	e := r.errorf(a.Span, "unknown annotation @%s", name)
	if s := didYouMean(name, cands); s != "" {
		e.Suggestion = s
	} else {
		e.Suggestion = "an annotation is named after a blueprint the app uses"
	}
}

func (r *resolver) annotationArgs(a *dsl.Annotation, spec *AnnotationSpec, name string, e *project.Entity) map[string]any {
	vals := map[string]any{}
	ok := true
	for _, arg := range a.Args {
		key := spec.Key(arg.Key.Name)
		if key == nil {
			keys := make([]string, len(spec.Keys))
			for i, k := range spec.Keys {
				keys[i] = k.Name
			}
			d := r.errorf(arg.Key.Span, "@%s has no key %q on %s", name, arg.Key.Name, plural(spec.Target))
			if s := didYouMean(arg.Key.Name, keys); s != "" {
				d.Suggestion = s
			} else if len(keys) > 0 {
				d.Suggestion = "keys are " + joinNames(keys)
			}
			ok = false
			continue
		}
		if _, dup := vals[key.Name]; dup {
			r.errorf(arg.Key.Span, "key %s is already set", key.Name)
			ok = false
			continue
		}
		if arg.Value == nil {
			if key.Type != TypeBool {
				r.errorf(arg.Key.Span, "key %s of @%s needs a value", key.Name, name).
					Suggestion = fmt.Sprintf("e.g. %s %s", key.Name, Example(key.Type))
				ok = false
				continue
			}
			vals[key.Name] = true
			continue
		}
		v, msg := Convert(*arg.Value, key.Type)
		if msg != "" {
			r.errorf(arg.Value.Span, "key %s of @%s: %s", key.Name, name, msg)
			ok = false
			continue
		}
		if key.Type == TypeField || key.Type == TypeFields {
			if !r.checkFieldNames(*arg.Value, e) {
				ok = false
				continue
			}
		}
		vals[key.Name] = v
	}
	for _, k := range spec.Keys {
		if _, set := vals[k.Name]; set {
			continue
		}
		switch {
		case k.Default != nil:
			vals[k.Name] = k.Default
		case k.Required:
			r.errorf(a.Span, "@%s needs key %s", name, k.Name).
				Suggestion = fmt.Sprintf("e.g. @%s(%s %s)", name, k.Name, Example(k.Type))
			ok = false
		}
	}
	if !ok {
		return nil
	}
	return vals
}

// checkFieldNames reports names in a field or fields value that are not
// fields (or foreign-key relations) of the entity.
func (r *resolver) checkFieldNames(v dsl.Value, e *project.Entity) bool {
	if e == nil {
		return true
	}
	var known []string
	for _, f := range e.Fields {
		known = append(known, f.Raw)
	}
	for _, rel := range e.Relations {
		if rel.OwnsForeignKey() {
			known = append(known, rel.Raw)
		}
	}
	items := []dsl.Value{v}
	if v.Kind == dsl.ListValue {
		items = v.List
	}
	ok := true
	for _, it := range items {
		if !slices.Contains(known, it.Text) {
			r.errorf(it.Span, "%s has no field %q", e.Raw, it.Text).Suggestion = didYouMean(it.Text, known)
			ok = false
		}
	}
	return ok
}

// Example is a value of type t, for messages and placeholders.
func Example(t ValueType) string {
	switch t {
	case TypeString:
		return `"text"`
	case TypeInt, TypeNumber:
		return "10"
	case TypeBool:
		return "true"
	case TypeList:
		return "[a, b]"
	case TypeFields:
		return "[name]"
	}
	return "name"
}

func plural(t Target) string {
	if t == TargetEntity {
		return "entities"
	}
	return string(t) + "s"
}

// refString writes a blueprint reference as in the app block.
func refString(ref *project.BlueprintRef) string {
	if ref.Source == project.SourceBuiltin {
		return ref.Ref
	}
	return `"` + ref.Ref + `"`
}
