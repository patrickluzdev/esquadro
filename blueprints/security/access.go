package security

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Access is who may do op (list, read, create, update or delete) on entity
// e: "public", a realm ("user") or a realm's role ("user.admin"). list
// falls back to read, and create, update and delete to write. An entity
// without any rule may be changed by whoever signed in, in any realm; one
// with rules, by nobody where it has none for op. Without realms, nil:
// nothing is checked.
func Access(entities []*project.Entity, e *project.Entity, op string) []string {
	realms := RealmsOf(entities)
	if len(realms) == 0 {
		return nil
	}
	a := e.Ann["security"]
	ruled := slices.ContainsFunc(Operations, func(o string) bool { return names(a[o]) != nil })
	if !ruled {
		out := make([]string, len(realms))
		for i, r := range realms {
			out[i] = r.Name
		}
		return out
	}
	who := names(a[op])
	switch {
	case who != nil:
	case op == "list":
		who = names(a["read"])
	case op == "create" || op == "update" || op == "delete":
		who = names(a["write"])
	}
	if who == nil {
		return []string{}
	}
	return who
}

// AccessText is Access as a sentence, for the API's documentation.
func AccessText(rules []string) string {
	if len(rules) == 0 {
		return "nobody"
	}
	parts := make([]string, len(rules))
	for i, r := range rules {
		realm, role, ok := strings.Cut(r, ".")
		switch {
		case r == "public":
			parts[i] = "anyone, signed in or not"
		case ok:
			parts[i] = "a " + realm + " with role " + role
		default:
			parts[i] = "any " + realm
		}
	}
	return strings.Join(parts, ", or ")
}
