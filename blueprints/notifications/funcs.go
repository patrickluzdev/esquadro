// Package notifications is the notifications blueprint's Go side: the
// rules of the model that send notices, for its templates and the store's.
package notifications

import (
	"slices"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Rule is what an entity's @notifications says: on which changes, about
// which fields, to whom, with what title and text.
type Rule struct {
	On, When, To []string
	Title, Text  string
}

// Has reports whether the rule notifies on action: create, update, delete.
func (r *Rule) Has(action string) bool { return slices.Contains(r.On, action) }

// Recipient is one of a rule's to: a realm and maybe a role, or the owner.
type Recipient struct {
	Owner       bool
	Realm, Role string
}

// Recipients are the rule's to, read.
func (r *Rule) Recipients() []Recipient {
	var out []Recipient
	for _, t := range r.To {
		if t == "owner" {
			out = append(out, Recipient{Owner: true})
			continue
		}
		realm, role, _ := strings.Cut(t, ".")
		out = append(out, Recipient{Realm: realm, Role: role})
	}
	return out
}

func names(v any) []string {
	switch l := v.(type) {
	case string:
		return []string{l}
	case []string:
		return l
	case []any:
		var out []string
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// RuleOf is the rule of an entity, nil without one.
func RuleOf(e *project.Entity) *Rule {
	a, ok := e.Ann["notifications"]
	if !ok {
		return nil
	}
	r := &Rule{On: names(a["on"]), When: names(a["when"]), To: names(a["to"])}
	r.Title, _ = a["title"].(string)
	r.Text, _ = a["text"].(string)
	return r
}

// TemplateFuncs are the functions of the notifications blueprint's templates.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"noticeRule": RuleOf,
		"realms":     security.RealmsOf,
		"noticeMoney": func(entities []*project.Entity) bool {
			for _, e := range Noticed(entities) {
				for _, f := range e.Fields {
					if f.Type == project.TypeMoney {
						return true
					}
				}
			}
			return false
		},
		"noticeEntities":  Noticed,
		"noticeOwner":     OwnerFor,
		"identitiesTable": func(pg map[string]any) string { return security.Table("identities", pg) },
		"rolesTable":      func(pg map[string]any) string { return security.Table("role_assignments", pg) },
	}
}

// OwnerFor is the owner of e's records, for to owner; nil without one.
func OwnerFor(entities []*project.Entity, e *project.Entity) *security.Owner {
	if o, why := security.OwnerOf(entities, e); why == "" {
		return &o
	}
	return nil
}

// Noticed are the entities with a rule.
func Noticed(entities []*project.Entity) []*project.Entity {
	var out []*project.Entity
	for _, e := range entities {
		if RuleOf(e) != nil {
			out = append(out, e)
		}
	}
	return out
}

// Check refuses rules that cannot send anything.
func Check(p *project.Project, _ *blueprint.Set) diag.List {
	var out diag.List
	realms := map[string][]string{}
	for _, r := range security.RealmsOf(p.Entities) {
		realms[r.Name] = r.Roles
	}
	for _, e := range p.Entities {
		r := RuleOf(e)
		if r == nil {
			continue
		}
		if len(r.On) == 0 || len(r.To) == 0 || r.Title == "" {
			out.Errorf(e.Pos, e.Pos, "@notifications on %s needs on, to and title", e.Raw)
			continue
		}
		for _, on := range r.On {
			if !slices.Contains([]string{"create", "update", "delete"}, on) {
				out.Errorf(e.Pos, e.Pos, "@notifications on %s: on is create, update or delete, not %s", e.Raw, on)
			}
		}
		if len(r.When) > 0 && !r.Has("update") {
			out.Errorf(e.Pos, e.Pos, "@notifications on %s: when is for on update", e.Raw)
		}
		for _, to := range r.Recipients() {
			if to.Owner {
				if _, why := security.OwnerOf(p.Entities, e); why != "" {
					out.Errorf(e.Pos, e.Pos, "@notifications on %s: to owner, but %s", e.Raw, why)
				}
				continue
			}
			roles, ok := realms[to.Realm]
			switch {
			case !ok:
				out.Errorf(e.Pos, e.Pos, "@notifications on %s: %s is no realm", e.Raw, to.Realm)
			case to.Role != "" && !slices.Contains(roles, to.Role):
				out.Errorf(e.Pos, e.Pos, "@notifications on %s: %s has no role %s", e.Raw, to.Realm, to.Role)
			}
		}
		for _, text := range []string{r.Title, r.Text} {
			for _, name := range placeholders(text) {
				if !slices.ContainsFunc(e.Fields, func(f *project.Field) bool { return f.Snake == name }) {
					out.Errorf(e.Pos, e.Pos, "@notifications on %s: {%s} is no field of it", e.Raw, name)
				}
			}
		}
	}
	return out
}

// placeholders are the {field} of a text.
func placeholders(text string) []string {
	var out []string
	for {
		i := strings.Index(text, "{")
		if i < 0 {
			return out
		}
		j := strings.Index(text[i:], "}")
		if j < 0 {
			return out
		}
		out = append(out, text[i+1:i+j])
		text = text[i+j+1:]
	}
}
