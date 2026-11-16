package security

import (
	"slices"
	"strings"
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

func TestOwnerOf(t *testing.T) {
	customer := &project.Entity{Ann: project.Ann{"security": {"principal": true}}}
	customer.Snake, customer.Raw = "customer", "Customer"
	tag := &project.Entity{}
	tag.Snake, tag.Raw = "tag", "Tag"
	relation := func(e, target *project.Entity, name string) *project.Relation {
		r := &project.Relation{Kind: project.ManyToOne, Entity: e, Target: target}
		r.Snake, r.Raw = name, name
		e.Relations = append(e.Relations, r)
		return r
	}
	order := &project.Entity{Ann: project.Ann{"security": {}}}
	order.Snake, order.Raw = "order", "Order"
	buyer := relation(order, customer, "buyer")
	relation(order, tag, "tag")
	all := []*project.Entity{customer, order, tag}

	if o, why := OwnerOf(all, order); why != "" || o.Relation != buyer || o.Realm != "customer" {
		t.Fatalf("one relation to a principal: %+v %s", o, why)
	}
	if o, why := OwnerOf(all, customer); why != "" || o.Relation != nil || o.Realm != "customer" {
		t.Fatalf("a principal owns itself: %+v %s", o, why)
	}
	if _, why := OwnerOf(all, tag); !strings.Contains(why, "needs a relation of Tag to a principal") {
		t.Fatal(why)
	}
	gift := relation(order, customer, "gifted_to")
	if _, why := OwnerOf(all, order); !strings.Contains(why, "more than one relation to a principal") {
		t.Fatal(why)
	}
	order.Ann["security"]["owner"] = "gifted_to"
	if o, _ := OwnerOf(all, order); o.Relation != gift {
		t.Fatal("owner names the relation")
	}
	order.Ann["security"]["owner"] = "nope"
	if _, why := OwnerOf(all, order); !strings.Contains(why, "owner nope is not a relation") {
		t.Fatal(why)
	}

	// write owner on a principal: update and delete, not create.
	customer.Ann["security"]["write"] = []any{"owner"}
	if got := Access(all, customer, "create"); len(got) != 0 {
		t.Fatalf("create: %v", got)
	}
	if got := Access(all, customer, "update"); !slices.Equal(got, []string{"owner"}) {
		t.Fatalf("update: %v", got)
	}
	if got := AccessText([]string{"owner", "user.admin"}); got != "the one it belongs to, or a user with role admin" {
		t.Error(got)
	}
}
