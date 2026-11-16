package security

import (
	"slices"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestAccess(t *testing.T) {
	entity := func(name string, ann map[string]any) *project.Entity {
		e := &project.Entity{Ann: project.Ann{"security": ann}}
		e.Snake = name
		return e
	}
	user := entity("user", map[string]any{"principal": true, "roles": []any{"admin"}, "read": []any{"user"}, "write": []any{"user.admin"}})
	customer := entity("customer", map[string]any{"principal": true})
	note := entity("note", map[string]any{"read": []any{"public"}, "delete": []any{"user.admin"}})
	tag := entity("tag", nil)
	all := []*project.Entity{user, customer, note, tag}

	for _, tc := range []struct {
		e    *project.Entity
		op   string
		want []string
	}{
		{user, "list", []string{"user"}},
		{user, "update", []string{"user.admin"}},
		{note, "list", []string{"public"}},
		{note, "delete", []string{"user.admin"}},
		{note, "create", []string{}},
		{tag, "update", []string{"user", "customer"}},
		{customer, "read", []string{"user", "customer"}},
	} {
		if got := Access(all, tc.e, tc.op); !slices.Equal(got, tc.want) || got == nil {
			t.Errorf("%s %s: %v, want %v", tc.e.Snake, tc.op, got, tc.want)
		}
	}
	if Access([]*project.Entity{note, tag}, note, "read") != nil {
		t.Error("without realms, nothing is checked")
	}
	if got := AccessText([]string{"public"}); got != "anyone, signed in or not" {
		t.Error(got)
	}
	if got := AccessText([]string{"user.admin", "customer"}); got != "a user with role admin, or any customer" {
		t.Error(got)
	}
	if got := AccessText(nil); got != "nobody" {
		t.Error(got)
	}
}
