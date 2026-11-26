package security

import (
	"slices"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestAccessTable(t *testing.T) {
	user := &project.Entity{Ann: project.Ann{"security": {"principal": true, "roles": []any{"admin", "manager"}, "read": []any{"user"}, "write": []any{"user.admin"}}}}
	user.Snake, user.Plural.Kebab = "user", "users"
	offer := &project.Entity{Ann: project.Ann{
		"security": {"read": []any{"public"}, "write": []any{"user.admin", "user.manager"}, "delete": []any{"user.admin"}},
		"store":    {"import": true, "soft_delete": true},
	}}
	offer.Snake, offer.Plural.Kebab = "offer", "offers"
	cost := &project.Field{Ann: project.Ann{"security": {"read": []any{"user.admin"}}}}
	cost.Snake = "cost"
	offer.Fields = []*project.Field{cost}
	settings := &project.Entity{Ann: project.Ann{"security": {"read": []any{"public"}, "write": []any{"user.admin"}}, "store": {"single": true}}}
	settings.Snake, settings.Kebab = "settings", "site-settings"
	report := &project.Report{Ann: project.Ann{"security": {"read": []any{"user.admin"}}}}
	report.Kebab = "top-offers"

	table := AccessTable([]*project.Entity{user, offer, settings}, []*project.Report{report})
	byName := map[string]AccessEntry{}
	for _, e := range table {
		byName[e.Name] = e
	}
	who := func(entry, op string) []string {
		for _, r := range byName[entry].Ops {
			if r.Name == op {
				return r.Who
			}
		}
		return nil
	}
	for _, tc := range []struct {
		entry, op string
		want      []string
	}{
		{"offers", "list", []string{"public"}},
		{"offers", "create", []string{"user.admin", "user.manager"}},
		{"offers", "delete", []string{"user.admin"}},
		{"offers", "import", []string{"user.admin", "user.manager"}},
		{"offers", "restore", []string{"user.admin"}},
		{"offers", "purge", []string{"user.admin"}},
		{"site-settings", "update", []string{"user.admin"}},
		{"reports/top-offers", "read", []string{"user.admin"}},
	} {
		if got := who(tc.entry, tc.op); !slices.Equal(got, tc.want) {
			t.Errorf("%s %s: %v, want %v", tc.entry, tc.op, got, tc.want)
		}
	}
	if who("site-settings", "delete") != nil || who("site-settings", "list") != nil {
		t.Error("an entity of one record has no list nor delete")
	}
	if f := byName["offers"].Fields; len(f) != 1 || f[0].Name != "cost" || !slices.Equal(f[0].Who, []string{"user.admin"}) {
		t.Errorf("the fields only some read: %+v", f)
	}
	if AccessTable([]*project.Entity{offer}, nil) != nil {
		t.Error("without realms, nothing is checked")
	}
}
