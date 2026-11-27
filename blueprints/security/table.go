package security

import (
	"slices"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// AccessEntry is what the API lets do with one of its resources, as the
// model's rules say: the session tells a frontend, so that it shows only
// what would not be refused. Name is the resource as the API's paths have
// it: "offers", "site-settings", "reports/top-offers"; and "accounts/user"
// for the administration of a realm's accounts.
type AccessEntry struct {
	Name string
	// Ops are each operation and who may: list, read, create, update,
	// delete; import, restore and purge where the store has them; and each
	// step of a flow, as a record's can says it ("status:PUBLISHED").
	Ops []AccessRule
	// Fields are the fields only some may read, and who.
	Fields []AccessRule
}

// AccessRule is an operation, or a field, and who may: the rules of
// @security ("public", "user", "user.admin", "owner").
type AccessRule struct {
	Name string
	Who  []string
}

// AccessTable is what the API lets do, resource by resource: the entities
// and the reports. Without realms, nil: nothing is checked.
func AccessTable(entities []*project.Entity, reports []*project.Report) []AccessEntry {
	if len(RealmsOf(entities)) == 0 {
		return nil
	}
	var out []AccessEntry
	for _, e := range entities {
		store := e.Ann["store"]
		single, _ := store["single"].(bool)
		entry := AccessEntry{Name: e.Plural.Kebab}
		ops := []string{"list", "read", "create", "update", "delete"}
		if single {
			entry.Name = e.Kebab
			ops = []string{"read", "update"}
		}
		for _, op := range ops {
			entry.Ops = append(entry.Ops, AccessRule{Name: op, Who: Access(entities, e, op)})
		}
		if imports, _ := store["import"].(bool); imports || store["import_by"] != nil {
			entry.Ops = append(entry.Ops, AccessRule{Name: "import", Who: Access(entities, e, "create")})
		}
		if trash, _ := store["soft_delete"].(bool); trash {
			del := Access(entities, e, "delete")
			entry.Ops = append(entry.Ops, AccessRule{Name: "restore", Who: del}, AccessRule{Name: "purge", Who: del})
		}
		update := Access(entities, e, "update")
		for _, fl := range Flows(e) {
			for _, to := range fl.Targets() {
				var who []string
				for _, m := range fl.Moves {
					if m.To != to {
						continue
					}
					by := m.By
					if len(by) == 0 {
						by = update
					}
					for _, w := range by {
						if !slices.Contains(who, w) {
							who = append(who, w)
						}
					}
				}
				entry.Ops = append(entry.Ops, AccessRule{Name: fl.Field.Snake + ":" + to.Raw, Who: who})
			}
		}
		for _, f := range e.Fields {
			if who := names(f.Ann["security"]["read"]); who != nil {
				entry.Fields = append(entry.Fields, AccessRule{Name: f.Snake, Who: who})
			}
		}
		out = append(out, entry)
	}
	// The accounts of each realm, for whoever administers them: inviting
	// (create), their roles, invitations and disabling (update), deleting.
	for _, r := range RealmsOf(entities) {
		admins := Admins(entities, r.Entity)
		if len(admins) == 0 {
			continue
		}
		entry := AccessEntry{Name: "accounts/" + r.Name}
		for _, op := range []string{"list", "create", "update", "delete"} {
			entry.Ops = append(entry.Ops, AccessRule{Name: op, Who: admins})
		}
		out = append(out, entry)
	}
	for _, r := range reports {
		who := names(r.Ann["security"]["read"])
		if who == nil {
			who = []string{}
		}
		out = append(out, AccessEntry{Name: "reports/" + r.Kebab, Ops: []AccessRule{{Name: "read", Who: who}}})
	}
	return out
}
