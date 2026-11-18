package security

import (
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers of the security blueprint's templates.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"securityTable": Table,
		"realms":        RealmsOf,
		"usesProvider":  UsesProvider,
		"access":        Access,
		"rulesOf":       RulesOf,
		"realmDisplay":  Display,
		"realmAdmins":   Admins,
		"serviceAccess": ServiceAccess,
		"whosOf":        WhosOf,
		"allowed":       Allowed,
		"listScope":     ListScope,
		"allowExpr":     AllowExpr,
		"list":          func(items ...string) []string { return items },
		"lower":         strings.ToLower,
		"goList": func(items []string) string {
			quoted := make([]string, len(items))
			for i, s := range items {
				quoted[i] = strconv.Quote(s)
			}
			return strings.Join(quoted, ", ")
		},
		"ownerOf": func(entities []*project.Entity, e *project.Entity) *Owner {
			if o, ok := OwnerUsed(entities, e); ok {
				return &o
			}
			return nil
		},
		"has": slices.Contains[[]string],
		// goElems is a []string literal without its type, for the values of
		// a map: {"a", "b"}.
		"goElems": func(items []string) string {
			quoted := make([]string, len(items))
			for i, s := range items {
				quoted[i] = strconv.Quote(s)
			}
			return "{" + strings.Join(quoted, ", ") + "}"
		},
		// manyToMany are the relations of e the API links and unlinks.
		"manyToMany": func(e *project.Entity) []*project.Relation {
			var out []*project.Relation
			for _, r := range e.Relations {
				if r.Kind == project.ManyToMany {
					out = append(out, r)
				}
			}
			return out
		},
	}
}

// UsesProvider reports whether a realm of entities signs in with provider.
func UsesProvider(entities []*project.Entity, provider string) bool {
	for _, r := range RealmsOf(entities) {
		if slices.Contains(r.Providers, provider) {
			return true
		}
	}
	return false
}

// Table is the name of one of the security tables: security_<name> in the
// public schema, or security.<name> when the postgres blueprint's schemas
// option has security.
func Table(name string, postgresOptions map[string]any) string {
	if slices.Contains(names(postgresOptions["schemas"]), "security") {
		return "security." + name
	}
	return "security_" + name
}

// Display is the column a realm's accounts are called by in their list: a
// field name, or the first text field.
func Display(e *project.Entity) string {
	for _, f := range e.Fields {
		if f.Snake == "name" {
			return f.Snake
		}
	}
	for _, f := range e.Fields {
		if !f.Implicit && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return f.Snake
		}
	}
	return ""
}

// Admins are the rules of who administers a realm's accounts: those its
// entity lets update it, but its owner (each account itself) and anyone.
func Admins(entities []*project.Entity, e *project.Entity) []string {
	out := []string{}
	for _, w := range RulesOf(entities, e, "update").Others {
		if w != "public" {
			out = append(out, w)
		}
	}
	return out
}
