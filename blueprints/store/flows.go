package store

import (
	"fmt"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// checkFlows checks @store(flow [...]): on an enum field with a default,
// each move "FROM -> TO" or "FROM -> TO by who", of values of the enum,
// the default the start of one, and who a realm or realm.role (or a
// service key's role) that exists.
func checkFlows(p *project.Project, set *blueprint.Set, diags *diag.List) {
	known := map[string][]string{} // realm -> its roles
	for _, r := range security.RealmsOf(p.Entities) {
		known[r.Name] = r.Roles
	}
	if services, _ := set.Options["security"]["services"].([]any); len(services) > 0 {
		for _, s := range services {
			known[security.ServiceRealm] = append(known[security.ServiceRealm], fmt.Sprint(s))
		}
	}
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			raw, ok := f.Ann["store"]["flow"].([]any)
			if !ok {
				continue
			}
			if f.Enum == nil {
				diags.Errorf(f.Pos, f.Pos, "flow: %s.%s is %s; a flow is of an enum field's values", e.Raw, f.Raw, f.Type)
				continue
			}
			if f.Default == nil {
				diags.Errorf(f.Pos, f.Pos, "flow: %s.%s has no default, the state its records start in: give it one, e.g. default(%s)", e.Raw, f.Raw, f.Enum.Values[0].Raw)
			}
			if len(raw) == 0 {
				diags.Errorf(f.Pos, f.Pos, "flow of %s.%s has no moves: write them as \"FROM -> TO\"", e.Raw, f.Raw)
				continue
			}
			var values []string
			for _, v := range f.Enum.Values {
				values = append(values, v.Raw)
			}
			starts := false
			seen := map[string]bool{}
			for _, x := range raw {
				s, _ := x.(string)
				m, ok := security.ParseMove(s)
				if !ok {
					diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: %q is not a move: write \"FROM -> TO\", or \"FROM -> TO by user.admin, user.moderator\"", e.Raw, f.Raw, s)
					continue
				}
				for _, state := range []string{m.From, m.To} {
					if !slices.Contains(values, state) {
						diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: %s is not a value of %s: %s", e.Raw, f.Raw, state, f.Enum.Raw, strings.Join(values, ", "))
					}
				}
				switch {
				case m.From == m.To:
					diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: %s goes to itself; staying is no move", e.Raw, f.Raw, s)
				case seen[m.From+"->"+m.To]:
					diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: %s -> %s is written twice", e.Raw, f.Raw, m.From, m.To)
				}
				seen[m.From+"->"+m.To] = true
				if f.Default != nil && m.From == f.Default.Text {
					starts = true
				}
				for _, w := range m.By {
					realm, role, hasRole := strings.Cut(w, ".")
					roles, ok := known[realm]
					switch {
					case len(known) == 0:
						diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: by %s needs a realm (an entity marked @security(principal)) of the security blueprint", e.Raw, f.Raw, w)
					case !ok:
						var realms []string
						for r := range known {
							realms = append(realms, r)
						}
						slices.Sort(realms)
						diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: by %s is not a realm: %s", e.Raw, f.Raw, w, strings.Join(realms, ", "))
					case hasRole && !slices.Contains(roles, role):
						diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: by %s: %s is not a role of %s: %s", e.Raw, f.Raw, w, role, realm, strings.Join(roles, ", "))
					}
				}
			}
			if f.Default != nil && !starts {
				diags.Errorf(f.Pos, f.Pos, "flow of %s.%s: its records start in %s, its default, which no move leaves: begin one there (\"%s -> ...\")", e.Raw, f.Raw, f.Default.Text, f.Default.Text)
			}
		}
	}
}
