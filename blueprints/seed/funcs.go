// Package seed holds the Go side of the seed blueprint: what its templates
// know of the model, and the checks of the project's seed files.
package seed

import (
	"slices"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the functions the seed blueprint's templates call.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"seedEntities": Ordered,
		"seedKeys":     Keys,
		"seedKind":     KindOf,
		"seedUses":     Uses,
	}
}

// Keys are the relations whose key an entity holds: what its records
// point at.
func Keys(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.OwnsForeignKey() {
			out = append(out, r)
		}
	}
	return out
}

// Ordered are the entities, each after the ones it points at (when it
// can), in the model's order otherwise: the order a seed creates them in,
// and deletes them backwards. A single entity is left out: its one record
// has its defaults.
func Ordered(entities []*project.Entity) []*project.Entity {
	var out []*project.Entity
	placed := map[*project.Entity]bool{}
	visiting := map[*project.Entity]bool{}
	var place func(e *project.Entity)
	place = func(e *project.Entity) {
		if placed[e] || visiting[e] {
			return
		}
		visiting[e] = true
		for _, r := range Keys(e) {
			if r.Target != e {
				place(r.Target)
			}
		}
		visiting[e] = false
		placed[e] = true
		out = append(out, e)
	}
	for _, e := range entities {
		if !store.Single(e) {
			place(e)
		}
	}
	return out
}

// KindOf is a field's kind, as the seed reads and makes it: its constant
// in the generated code.
func KindOf(f *project.Field) string {
	switch f.Type {
	case project.TypeText:
		return "Text"
	case project.TypeInt:
		return "Int"
	case project.TypeInt64:
		return "Int64"
	case project.TypeDecimal:
		return "Decimal"
	case project.TypeMoney:
		return "Money"
	case project.TypeBool:
		return "Bool"
	case project.TypeTime:
		return "Time"
	case project.TypeDate:
		return "Date"
	case project.TypeUUID:
		return "UUID"
	case project.TypeJSON:
		return "JSON"
	case project.TypeEnum:
		return "Enum"
	case project.TypeFile, project.TypeImage:
		return "File"
	}
	return "String"
}

// Uses reports whether a field of one of the entities has the type, for
// the generated code's imports and helpers.
func Uses(entities []*project.Entity, t string) bool {
	return slices.ContainsFunc(entities, func(e *project.Entity) bool {
		return slices.ContainsFunc(e.Fields, func(f *project.Field) bool { return !f.Implicit && string(f.Type) == t })
	})
}
