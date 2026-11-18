package store

import (
	"slices"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// SoftDeleted reports whether deleting a record of the entity puts it in
// the trash (@store(soft_delete)) instead of removing it.
func SoftDeleted(e *project.Entity) bool {
	on, _ := e.Ann["store"]["soft_delete"].(bool)
	return on
}

// Dependent is a relation of another entity of the module that points at
// one with a trash: what putting a record in the trash does to the records
// that point at it.
type Dependent struct {
	// Relation is the side that holds the key; Relation.Entity, the child.
	Relation *project.Relation
	// Action is "cascade", "restrict" or "set_null", as the key's ON DELETE.
	Action string
	// Soft tells whether the child has a trash too.
	Soft bool
}

// Name is how the queries about the dependent are called: Product and
// Category give ProductByCategory.
func (d Dependent) Name() string { return d.Relation.Entity.Pascal + "By" + d.Relation.Pascal }

// Column is the child's key column.
func (d Dependent) Column() string { return d.Relation.Snake + "_id" }

// Dependents are the relations of the module's entities that point at e
// with a foreign key, for its trash. Relations from other modules have no
// key, and deleting never looked at them.
func Dependents(e *project.Entity) []Dependent {
	var out []Dependent
	for _, other := range e.Module.Entities {
		for _, r := range other.Relations {
			if r.Target != e || !r.OwnsForeignKey() {
				continue
			}
			out = append(out, Dependent{Relation: r, Action: OnDelete(r), Soft: SoftDeleted(other)})
		}
	}
	return out
}

// TrashedParents are e's relations, with a key, to an entity of its module
// with a trash: a record may not point at one in the trash.
func TrashedParents(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.OwnsForeignKey() && !r.CrossesModules() && SoftDeleted(r.Target) {
			out = append(out, r)
		}
	}
	return out
}

// OnDelete is what deleting the target does to the records of a relation
// that holds the key: "cascade", "restrict" or "set_null", as declared, or
// restrict when it is required and set_null when not.
func OnDelete(r *project.Relation) string {
	switch {
	case r.OnDelete != "":
		return r.OnDelete
	case r.Required:
		return "restrict"
	}
	return "set_null"
}

// TrashModules are the modules with an entity with a trash, for the purge.
func TrashModules(modules []*project.Module) []*project.Module {
	var out []*project.Module
	for _, m := range modules {
		if slices.ContainsFunc(m.Entities, SoftDeleted) {
			out = append(out, m)
		}
	}
	return out
}
