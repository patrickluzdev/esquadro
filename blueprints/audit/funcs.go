// Package audit is the audit blueprint's Go side: which entities and fields
// are audited, for its templates and the store's.
package audit

import (
	"slices"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the functions of the audit blueprint's templates.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"audited":      Audited,
		"auditedOf":    AuditedOf,
		"auditSecret":  Secret,
		"auditIgnored": Ignored,
		"auditAdmins":  Admins,
		"auditMoney": func(entities []*project.Entity, opts map[string]any) bool {
			for _, e := range AuditedOf(entities, opts) {
				for _, f := range e.Fields {
					if f.Type == project.TypeMoney {
						return true
					}
				}
			}
			return false
		},
	}
}

// Audited reports whether the changes of e are kept: with the blueprint
// (opts are its options, nil without it), e marked @audit, or every entity
// with entities all, but those marked @audit(off).
func Audited(e *project.Entity, opts map[string]any) bool {
	if opts == nil {
		return false
	}
	a, marked := e.Ann["audit"]
	if off, _ := a["off"].(bool); off {
		return false
	}
	if all, _ := opts["entities"].(string); all == "all" {
		return true
	}
	return marked
}

// AuditedOf are the entities of a list whose changes are kept.
func AuditedOf(entities []*project.Entity, opts map[string]any) []*project.Entity {
	var out []*project.Entity
	for _, e := range entities {
		if Audited(e, opts) {
			out = append(out, e)
		}
	}
	return out
}

// Admins are who see every change: the admins option, or those who
// administer the accounts of the first realm.
func Admins(entities []*project.Entity, opts map[string]any) []string {
	var out []string
	switch l := opts["admins"].(type) {
	case []string:
		out = append(out, l...)
	case []any:
		for _, v := range l {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		if realms := security.RealmsOf(entities); len(realms) > 0 {
			out = security.Admins(entities, realms[0].Entity)
		}
	}
	return out
}

// Secret reports whether a field's changes are kept without its values.
func Secret(f *project.Field) bool {
	on, _ := f.Ann["audit"]["secret"].(bool)
	return on
}

// Ignored reports whether a field's changes are not kept.
func Ignored(f *project.Field) bool {
	on, _ := f.Ann["audit"]["ignore"].(bool)
	return on
}

// Check refuses options the blueprint does not know the values of.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var out diag.List
	opts := set.Options["audit"]
	if opts == nil {
		return nil
	}
	for _, o := range []struct {
		key     string
		allowed []string
	}{{"entities", []string{"marked", "all"}}, {"history", []string{"update", "read", "admins"}}} {
		if v, _ := opts[o.key].(string); !slices.Contains(o.allowed, v) {
			out.Errorf(p.App.Pos, p.App.Pos, "audit: %s is %q, and is one of %s", o.key, v, strings.Join(o.allowed, ", "))
		}
	}
	return out
}
