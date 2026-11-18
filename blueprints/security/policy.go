package security

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Rules are an action's rules for a policy: the owner rule apart.
type Rules struct {
	// Others are public, realms and realm.roles.
	Others []string
	// Owner is whether the owner rule lets in too.
	Owner bool
}

// RulesOf are the rules of op on e: list, read, create, update or delete.
func RulesOf(entities []*project.Entity, e *project.Entity, op string) Rules {
	var r Rules
	for _, w := range Access(entities, e, op) {
		if w == "owner" {
			r.Owner = true
		} else {
			r.Others = append(r.Others, w)
		}
	}
	return r
}

// Who is someone a generated test asks a policy as: no one, a principal of
// a realm with roles, owning the record or not.
type Who struct {
	Name  string
	Realm string
	Roles []string
	Owns  bool
}

// WhosOf are the kinds of requests the tests of e's policy ask as: no one;
// for each realm, without roles and with each role; and, for the owner's
// realm, the owner and someone else.
//
// services are the service keys' roles, from the blueprint's services
// option: a service with each role asks too.
func WhosOf(entities []*project.Entity, e *project.Entity, services ...any) []Who {
	out := []Who{{Name: "no one"}}
	for _, s := range services {
		for _, role := range names(s) {
			out = append(out, Who{Name: ServiceRealm + "." + role, Realm: ServiceRealm, Roles: []string{role}})
		}
	}
	owner, hasOwner := OwnerUsed(entities, e)
	for _, r := range RealmsOf(entities) {
		out = append(out, Who{Name: r.Name, Realm: r.Name})
		for _, role := range r.Roles {
			out = append(out, Who{Name: r.Name + "." + role, Realm: r.Name, Roles: []string{role}})
		}
		if hasOwner && owner.Realm == r.Name {
			out = append(out, Who{Name: r.Name + " owner", Realm: r.Name, Owns: true})
		}
	}
	return out
}

// ListScope is what a list answers who: "all", "owned" (the records who
// owns) or "nobody".
func ListScope(entities []*project.Entity, e *project.Entity, who Who) string {
	// Allowed without owning: the rules but owner.
	if Allowed(entities, e, "list", Who{Realm: who.Realm, Roles: who.Roles}) {
		return "all"
	}
	if owner, ok := OwnerUsed(entities, e); ok && RulesOf(entities, e, "list").Owner && who.Realm == owner.Realm {
		return "owned"
	}
	return "nobody"
}

// AllowExpr is the Go expression of rules, in a policy's method: owns is
// the call that says whether the request comes from the record's owner.
func AllowExpr(rules Rules, owns string) string {
	var parts []string
	if len(rules.Others) > 0 {
		quoted := make([]string, len(rules.Others))
		for i, w := range rules.Others {
			quoted[i] = `"` + w + `"`
		}
		parts = append(parts, "security.Allows(ctx, "+strings.Join(quoted, ", ")+")")
	}
	if rules.Owner {
		parts = append(parts, owns)
	}
	if len(parts) == 0 {
		return "false"
	}
	return strings.Join(parts, " || ")
}

// Allowed is what op's rules answer to who: the expectation of the
// generated tests, worked out here rather than by the generated code.
func Allowed(entities []*project.Entity, e *project.Entity, op string, who Who) bool {
	rules := RulesOf(entities, e, op)
	if slices.Contains(rules.Others, "public") {
		return true
	}
	if who.Realm == "" {
		return false
	}
	for _, w := range rules.Others {
		realm, role, hasRole := strings.Cut(w, ".")
		if realm == who.Realm && (!hasRole || slices.Contains(who.Roles, role)) {
			return true
		}
	}
	if rules.Owner && who.Owns {
		return true
	}
	return false
}
