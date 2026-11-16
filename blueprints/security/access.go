package security

import (
	"fmt"
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
		// A principal is created by sign-up: write owner is its update and
		// delete.
		if principal, _ := a["principal"].(bool); principal && op == "create" {
			who = slices.DeleteFunc(slices.Clone(who), func(w string) bool { return w == "owner" })
		}
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
		case r == "owner":
			parts[i] = "the one it belongs to"
		case ok:
			parts[i] = "a " + realm + " with role " + role
		default:
			parts[i] = "any " + realm
		}
	}
	return strings.Join(parts, ", or ")
}

// Owner is whom a record of an entity belongs to, for the owner rule: the
// principal a relation points at, or, in a principal, the record itself.
type Owner struct {
	// Relation is the relation to the owner; nil for a principal, which
	// owns itself.
	Relation *project.Relation
	// Realm is the owner's realm.
	Realm string
}

// OwnerOf is the owner of e's records, and, when it has none, why: no
// relation to a principal, more than one without owner naming one, or an
// owner naming no such relation.
func OwnerOf(entities []*project.Entity, e *project.Entity) (Owner, string) {
	if principal, _ := e.Ann["security"]["principal"].(bool); principal {
		return Owner{Realm: e.Snake}, ""
	}
	realms := map[*project.Entity]string{}
	for _, r := range RealmsOf(entities) {
		realms[r.Entity] = r.Name
	}
	var candidates []*project.Relation
	for _, r := range e.Relations {
		if _, ok := realms[r.Target]; ok && r.OwnsForeignKey() {
			candidates = append(candidates, r)
		}
	}
	if named, _ := e.Ann["security"]["owner"].(string); named != "" {
		for _, r := range candidates {
			if r.Snake == named || r.Raw == named {
				return Owner{Relation: r, Realm: realms[r.Target]}, ""
			}
		}
		return Owner{}, fmt.Sprintf("owner %s is not a relation of %s to a principal", named, e.Raw)
	}
	switch len(candidates) {
	case 0:
		return Owner{}, fmt.Sprintf("the owner rule needs a relation of %s to a principal (an entity marked @security(principal)), e.g. customer > Customer", e.Raw)
	case 1:
		return Owner{Relation: candidates[0], Realm: realms[candidates[0].Target]}, ""
	}
	names := make([]string, len(candidates))
	for i, r := range candidates {
		names[i] = r.Raw
	}
	return Owner{}, fmt.Sprintf("%s has more than one relation to a principal (%s): say which one owns it, e.g. @security(owner %s, ...)", e.Raw, strings.Join(names, ", "), names[0])
}

// OwnerUsed is the owner of e's records when one of its rules is owner.
func OwnerUsed(entities []*project.Entity, e *project.Entity) (Owner, bool) {
	for _, op := range []string{"list", "read", "create", "update", "delete"} {
		if slices.Contains(Access(entities, e, op), "owner") {
			o, why := OwnerOf(entities, e)
			return o, why == ""
		}
	}
	return Owner{}, false
}
