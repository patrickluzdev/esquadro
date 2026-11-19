package store

import (
	"fmt"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/postgres"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Total is a field the store keeps (@store(sum votes.value) or
// @store(count comments)): the sum of a field, or the count, of the records
// of a one-to-many relation, worked out again in the transaction of every
// change of one of them. Writes leave it out.
type Total struct {
	// Field is the total, on the parent.
	Field *project.Field
	// Kind is sum or count.
	Kind string
	// Relation is the parent's one-to-many relation (votes); Key, the
	// child's side, which holds the key (Vote.offer).
	Relation, Key *project.Relation
	// Summed is the child's field a sum adds up; nil for a count.
	Summed *project.Field
}

// IsTotal reports whether a field is a total the store keeps, which writes
// leave out.
func IsTotal(f *project.Field) bool {
	a := f.Ann["store"]
	return a["sum"] != nil || a["count"] != nil
}

// Written reports whether a field is one a record is written with: declared,
// and not a total.
func Written(f *project.Field) bool { return !f.Implicit && !IsTotal(f) }

// totalOf resolves the total of a field of e; ok is false when the field is
// none or names what e does not have.
func totalOf(e *project.Entity, f *project.Field) (Total, bool) {
	a := f.Ann["store"]
	t := Total{Field: f}
	var path string
	if s, ok := a["sum"].(string); ok {
		t.Kind, path = "sum", s
	} else if s, ok := a["count"].(string); ok {
		t.Kind, path = "count", s
	} else {
		return t, false
	}
	rel, field, _ := strings.Cut(path, ".")
	for _, r := range e.Relations {
		if r.Raw == rel && r.Kind == project.OneToMany && r.Inverse != nil {
			t.Relation, t.Key = r, r.Inverse
		}
	}
	if t.Relation == nil {
		return t, false
	}
	if t.Kind == "sum" {
		t.Summed = t.Relation.Target.Field(field)
		if t.Summed == nil {
			return t, false
		}
	}
	return t, true
}

// Totals are the totals of e's fields.
func Totals(e *project.Entity) []Total {
	var out []Total
	for _, f := range e.Fields {
		if t, ok := totalOf(e, f); ok {
			out = append(out, t)
		}
	}
	return out
}

// TotalGroup is the totals of a parent over one of its relations, worked
// out again together: Recount<Parent><Relation>.
type TotalGroup struct {
	Relation, Key *project.Relation
	Totals        []Total
}

// Name names its queries: OfferVotes.
func (g TotalGroup) Name() string { return g.Relation.Entity.Pascal + g.Relation.Pascal }

// Parent is the entity that has the totals.
func (g TotalGroup) Parent() *project.Entity { return g.Relation.Entity }

// Column is the child's key column, which points at the parent.
func (g TotalGroup) Column() string { return postgres.Ident(g.Key.Snake + "_id") }

// TotalGroups are e's totals, by the relation they count over.
func TotalGroups(e *project.Entity) []TotalGroup {
	var out []TotalGroup
	for _, t := range Totals(e) {
		i := slices.IndexFunc(out, func(g TotalGroup) bool { return g.Relation == t.Relation })
		if i < 0 {
			out = append(out, TotalGroup{Relation: t.Relation, Key: t.Key})
			i = len(out) - 1
		}
		out[i].Totals = append(out[i].Totals, t)
	}
	return out
}

// CountedBy are the totals of other records that count c's: the groups
// whose children are c's records.
func CountedBy(c *project.Entity) []TotalGroup {
	var out []TotalGroup
	for _, e := range c.Module.Entities {
		for _, g := range TotalGroups(e) {
			if g.Key.Entity == c {
				out = append(out, g)
			}
		}
	}
	return out
}

// Cascaded is a total that deleting a record of another entity changes:
// the record's dependents of Child go with it (their relation Via
// cascades), and they count in Group.
type Cascaded struct {
	Via   *project.Relation
	Group TotalGroup
}

// Name names the query of the parents to work out again: OfferIDsOfVoteByVoter.
func (c Cascaded) Name() string {
	return c.Group.Parent().Pascal + "IDsOf" + c.Via.Entity.Pascal + "By" + c.Via.Pascal
}

// Cascades are the totals that deleting a record of e changes through the
// records the database deletes with it: those of its module whose relation
// to e cascades, when other records count them.
func Cascades(e *project.Entity) []Cascaded {
	var out []Cascaded
	for _, d := range Dependents(e) {
		if d.Action != "cascade" {
			continue
		}
		for _, g := range CountedBy(d.Relation.Entity) {
			if g.Key != d.Relation {
				out = append(out, Cascaded{Via: d.Relation, Group: g})
			}
		}
	}
	return out
}

// TotalSQL is the SQL expression that works a total out for the parent
// @id: a sum or a count of the live records that point at it.
func TotalSQL(t Total, opts map[string]any) string {
	what := "count(*)"
	if t.Kind == "sum" {
		what = "coalesce(sum(c." + postgres.Ident(pgColumn(t.Summed)) + "), 0)"
	}
	where := "c." + postgres.Ident(t.Key.Snake+"_id") + " = sqlc.arg(id)::uuid"
	if SoftDeleted(t.Key.Entity) {
		where += " AND c.deleted_at IS NULL"
	}
	return "(SELECT " + what + " FROM " + pgTable(t.Key.Entity, opts) + " c WHERE " + where + ")"
}

// Order is an order of an entity's lists worked out by an SQL expression
// (@store(order [hot, "temperature / ..."])): sort=hot lists the largest
// first, -hot the smallest. A list in it pages by number only, since its
// values change with time.
type Order struct {
	project.Names
	// Expr is the SQL expression, over the entity's columns.
	Expr string
}

// Orders are e's orders, in the order written.
func Orders(e *project.Entity) []Order {
	list, _ := e.Ann["store"]["order"].([]any)
	var out []Order
	for i := 0; i+1 < len(list); i += 2 {
		name, _ := list[i].(string)
		expr, _ := list[i+1].(string)
		n, ok := project.NamesOf(name)
		if !ok || expr == "" {
			continue
		}
		out = append(out, Order{Names: n, Expr: expr})
	}
	return out
}

// OrderKeys are the sorts e's orders give: hot and -hot.
func OrderKeys(e *project.Entity) []string {
	var out []string
	for _, o := range Orders(e) {
		out = append(out, o.Snake, "-"+o.Snake)
	}
	return out
}

// totalTypes are the types a total may have, by what it adds up: a count
// is a whole number, a sum the type of what it adds (or int64, of int).
var totalTypes = map[project.Type][]project.Type{
	"count":             {project.TypeInt, project.TypeInt64},
	project.TypeInt:     {project.TypeInt, project.TypeInt64},
	project.TypeInt64:   {project.TypeInt64},
	project.TypeDecimal: {project.TypeDecimal},
}

// checkTotals checks @store(sum) and @store(count): a one-to-many relation
// of the entity, a number to add up, a total of a type that holds it, and
// not both on one field; and @store(order): pairs of a name, not one of a
// field, and an expression.
func checkTotals(p *project.Project, diags *diag.List) {
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			a := f.Ann["store"]
			if a["sum"] != nil && a["count"] != nil {
				diags.Errorf(f.Pos, f.Pos, "%s.%s is both a sum and a count: choose one", e.Raw, f.Raw)
				continue
			}
			if !IsTotal(f) {
				continue
			}
			kind, path := "count", ""
			if s, ok := a["sum"].(string); ok {
				kind, path = "sum", s
			} else {
				path, _ = a["count"].(string)
			}
			rel, field, dotted := strings.Cut(path, ".")
			r := relationNamed(e, rel)
			switch {
			case r == nil:
				diags.Errorf(f.Pos, f.Pos, "%s %s: %s has no relation %s", kind, path, e.Raw, rel)
				continue
			case r.Kind != project.OneToMany || r.Inverse == nil:
				diags.Errorf(f.Pos, f.Pos, "%s %s: %s.%s is %s; a total counts the records of a one-to-many relation (%s < Child)", kind, path, e.Raw, r.Raw, r.Kind, r.Raw)
				continue
			case kind == "count" && dotted:
				diags.Errorf(f.Pos, f.Pos, "count %s: a count names the relation only (count %s)", path, rel)
				continue
			case kind == "sum" && !dotted:
				diags.Errorf(f.Pos, f.Pos, "sum %s: a sum names the field it adds up (sum %s.<field>)", path, rel)
				continue
			}
			if f.Required || f.Default != nil || f.Unique {
				diags.Errorf(f.Pos, f.Pos, "%s.%s is a total the store keeps: it takes no required, default or unique", e.Raw, f.Raw)
			}
			of := project.Type("count")
			if kind == "sum" {
				summed := r.Target.Field(field)
				if summed == nil || summed.Implicit {
					diags.Errorf(f.Pos, f.Pos, "sum %s: %s has no field %s", path, r.Target.Raw, field)
					continue
				}
				if _, ok := totalTypes[summed.Type]; !ok {
					diags.Errorf(f.Pos, f.Pos, "sum %s: %s.%s is %s; a sum adds up int, int64 or decimal", path, r.Target.Raw, summed.Raw, summed.Type)
					continue
				}
				if IsTotal(summed) {
					diags.Errorf(f.Pos, f.Pos, "sum %s: %s.%s is a total itself; a total adds up fields written", path, r.Target.Raw, summed.Raw)
					continue
				}
				of = summed.Type
			}
			if !slices.Contains(totalTypes[of], f.Type) {
				var want []string
				for _, t := range totalTypes[of] {
					want = append(want, string(t))
				}
				what := "a count"
				if kind == "sum" {
					what = fmt.Sprintf("a sum of %s", of)
				}
				diags.Errorf(f.Pos, f.Pos, "%s.%s is %s; %s is %s", e.Raw, f.Raw, f.Type, what, strings.Join(want, " or "))
			}
		}
		raw, _ := e.Ann["store"]["order"].([]any)
		if raw == nil {
			continue
		}
		if len(raw)%2 != 0 {
			diags.Errorf(e.Pos, e.Pos, "order of %s: write a name and its SQL expression for each order: order [hot, \"temperature / ...\"]", e.Raw)
			continue
		}
		seen := map[string]bool{}
		for i := 0; i < len(raw); i += 2 {
			name, isName := raw[i].(string)
			expr, isExpr := raw[i+1].(string)
			n, ok := project.NamesOf(name)
			switch {
			case !isName || !isExpr || !ok || strings.TrimSpace(expr) == "":
				diags.Errorf(e.Pos, e.Pos, "order of %s: write a name and its SQL expression for each order: order [hot, \"temperature / ...\"]", e.Raw)
				continue
			case e.Field(name) != nil || slices.ContainsFunc(e.Fields, func(f *project.Field) bool { return f.Snake == n.Snake }):
				diags.Errorf(e.Pos, e.Pos, "order %s of %s has the name of a field: name it apart (sort=%s would be the field's)", name, e.Raw, n.Snake)
			case n.Snake == "created_at" || n.Snake == "relevance":
				diags.Errorf(e.Pos, e.Pos, "order %s of %s has the name of a sort lists have already: name it apart", name, e.Raw)
			case seen[n.Snake]:
				diags.Errorf(e.Pos, e.Pos, "order %s of %s is written twice", name, e.Raw)
			case strings.Contains(expr, ";"):
				diags.Errorf(e.Pos, e.Pos, "order %s of %s: the expression is one SQL expression, without ;", name, e.Raw)
			}
			seen[n.Snake] = true
		}
		if Single(e) {
			diags.Errorf(e.Pos, e.Pos, "%s is single and has orders: its one record is never listed", e.Raw)
		}
	}
}

// relationNamed is e's relation with the given name as written, or nil.
func relationNamed(e *project.Entity, name string) *project.Relation {
	for _, r := range e.Relations {
		if r.Raw == name {
			return r
		}
	}
	return nil
}
