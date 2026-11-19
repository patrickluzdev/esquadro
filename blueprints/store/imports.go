package store

import (
	"slices"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// ImportMax is how many rows one import takes at most.
const ImportMax = 5000

// Importable reports whether records of the entity may be imported in a
// batch (@store(import) or @store(import_by field)).
func Importable(e *project.Entity) bool {
	on, _ := e.Ann["store"]["import"].(bool)
	return on || ImportBy(e) != nil
}

// ImportBy is the field an import knows a record by (@store(import_by
// field)): a row with the value of one updates it, others are created. Nil
// when an import only creates.
func ImportBy(e *project.Entity) *project.Field {
	name, _ := e.Ann["store"]["import_by"].(string)
	if name == "" {
		return nil
	}
	return e.Field(name)
}

// importKey is the field an import knows records by, as the queries
// write and take it.
func importKey(e *project.Entity) *Writable {
	f := ImportBy(e)
	if f == nil {
		return nil
	}
	for _, w := range uniqueKeys(e) {
		if w.Field == f {
			return &w
		}
	}
	return nil
}

// uniqueKeys are the fields a record may be read by, being unique, of a
// type that names a record: Read<Entity>By<Field>. A single entity has
// none: it is read without.
func uniqueKeys(e *project.Entity) []Writable {
	if Single(e) {
		return nil
	}
	var out []Writable
	for _, w := range writable(e) {
		if w.Field != nil && !w.Currency && w.Field.Unique && slices.Contains(importKinds, w.Field.Type) {
			out = append(out, w)
		}
	}
	return out
}

// Redirects reports whether the entity's records redirect on a site
// (@<site>(redirect)): its rules have BeforeRedirect and Visited.
func Redirects(e *project.Entity) bool {
	for _, a := range e.Ann {
		if on, _ := a["redirect"].(bool); on {
			return true
		}
	}
	return false
}

// RedirectTo is the field a redirecting entity's records send to, as its
// site says (@<site>(redirect, to target_url)).
func RedirectTo(e *project.Entity) *project.Field {
	for _, a := range e.Ann {
		if on, _ := a["redirect"].(bool); on {
			if name, _ := a["to"].(string); name != "" {
				return e.Field(name)
			}
		}
	}
	return nil
}

// RedirectModule reports whether a module has an entity that redirects:
// it has Visit and ErrGone.
func RedirectModule(m *project.Module) bool {
	return slices.ContainsFunc(m.Entities, Redirects)
}

// importKinds are the types a field an import knows records by may have.
var importKinds = []project.Type{project.TypeString, project.TypeText, project.TypeInt, project.TypeInt64, project.TypeUUID, project.TypeDate}

// checkImports checks @store(import_by): a field of the entity, unique and
// required, of a type that names a record; and no import of a single
// entity.
func checkImports(p *project.Project, diags *diag.List) {
	for _, e := range p.Entities {
		name, _ := e.Ann["store"]["import_by"].(string)
		if Importable(e) && Single(e) {
			diags.Errorf(e.Pos, e.Pos, "%s is single and imported: its one record is changed, never imported", e.Raw)
		}
		if name == "" {
			continue
		}
		f := e.Field(name)
		switch {
		case f == nil || f.Implicit:
			diags.Errorf(e.Pos, e.Pos, "import_by: %s is not a field of %s", name, e.Raw)
		case !f.Unique || !f.Required:
			diags.Errorf(f.Pos, f.Pos, "import_by %s: %s.%s must be required and unique, for an import to know a record by it", name, e.Raw, f.Raw)
		case !slices.Contains(importKinds, f.Type):
			diags.Errorf(f.Pos, f.Pos, "import_by %s: %s.%s is %s; an import knows records by a string, text, int, uuid or date", name, e.Raw, f.Raw, f.Type)
		}
	}
}
