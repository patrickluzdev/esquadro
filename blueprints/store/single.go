package store

import (
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// SingleID is the id of the one record of an entity with @store(single).
const SingleID = "00000000-0000-0000-0000-000000000001"

// Single reports whether the entity has one record only (@store(single)):
// it always exists, with its defaults until someone changes it, and is
// read and changed but never listed, created or deleted.
func Single(e *project.Entity) bool {
	on, _ := e.Ann["store"]["single"].(bool)
	return on
}

// checkSingle checks what an entity with one record only may not have:
// a required field without a default (there would be no value before the
// first save), a trash, relations (either way) and pages.
func checkSingle(p *project.Project, diags *diag.List) {
	for _, e := range p.Entities {
		if !Single(e) {
			continue
		}
		if SoftDeleted(e) {
			diags.Errorf(e.Pos, e.Pos, "%s is single and soft_delete: its one record is never deleted", e.Raw)
		}
		for _, f := range e.Fields {
			if !f.Implicit && f.Required && f.Default == nil {
				diags.Errorf(f.Pos, f.Pos, "%s.%s is required in a single entity: give it a default, the value it has before anyone saves", e.Raw, f.Raw)
			}
		}
		for _, r := range e.Relations {
			if r.Inferred {
				continue // the side of one that points at it: refused below
			}
			diags.Errorf(r.Pos, r.Pos, "%s is single and has the relation %s: a single entity has no relations; keep an id in a uuid field", e.Raw, r.Raw)
		}
		for name, a := range e.Ann {
			if on, _ := a["page"].(bool); on {
				diags.Errorf(e.Pos, e.Pos, "%s is single and has @%s(page): its one record has no page of its own", e.Raw, name)
			}
		}
		if on, _ := e.Ann["security"]["principal"].(bool); on {
			diags.Errorf(e.Pos, e.Pos, "%s is single and a principal: a principal has many records", e.Raw)
		}
	}
	for _, e := range p.Entities {
		for _, r := range e.Relations {
			if r.Target != nil && Single(r.Target) && !r.Inferred && !Single(e) {
				diags.Errorf(r.Pos, r.Pos, "%s.%s points at %s, which is single: a single entity has no relations; read it with Read%s", e.Raw, r.Raw, r.Target.Raw, r.Target.Pascal)
			}
		}
	}
}
