package rest

import (
	"slices"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// ReadLimit is a field only some may read, @security(read [...]) on it: the
// API leaves it out of the answers to the others, and refuses them what
// would tell its value, a filter or a sort by it.
type ReadLimit struct {
	Field *project.Field
	// Rules are who may read it, as security.Allows takes them.
	Rules []string
	// Params are the list's filters by it.
	Params []APIFilter
	// Sorts are the list's sorts by it, both ways.
	Sorts []string
}

// ReadLimits are e's fields only some may read.
func ReadLimits(e *project.Entity, opts map[string]any) []ReadLimit {
	var out []ReadLimit
	for _, f := range e.Fields {
		rules := strs(f.Ann["security"]["read"])
		if len(rules) == 0 {
			continue
		}
		l := ReadLimit{Field: f, Rules: rules}
		for _, a := range APIFilters(e) {
			if a.Field == f.Pascal {
				l.Params = append(l.Params, a)
			}
		}
		for _, s := range store.ListSorts(e, opts) {
			if s.Key == f.Snake {
				l.Sorts = append(l.Sorts, s.Key, "-"+s.Key)
			}
		}
		out = append(out, l)
	}
	return out
}

// anyReadLimited reports whether one of entities has a field only some may
// read.
func anyReadLimited(entities []*project.Entity) bool {
	for _, e := range entities {
		if readLimited(e) {
			return true
		}
	}
	return false
}

// readLimited reports whether e has a field only some may read.
func readLimited(e *project.Entity) bool {
	return slices.ContainsFunc(e.Fields, func(f *project.Field) bool { return len(readRules(f)) > 0 })
}

// readRules are who may read f, none when anyone who reads its record may.
func readRules(f *project.Field) []string { return strs(f.Ann["security"]["read"]) }
