package security

import (
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/internal/compose"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers of the security blueprint's templates.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"securityTable": Table,
		"resetPages":    ResetPages,
		"realms":        RealmsOf,
		"usesProvider":  UsesProvider,
		"access":        Access,
		"rulesOf":       RulesOf,
		"realmDisplay":  Display,
		"realmAdmins":   Admins,
		"serviceAccess": ServiceAccess,
		"whosOf":        WhosOf,
		"allowed":       Allowed,
		"listScope":     ListScope,
		"allowExpr":     AllowExpr,
		"flowsBy":       FlowsBy,
		"list":          func(items ...string) []string { return items },
		"lower":         strings.ToLower,
		"goList": func(items []string) string {
			quoted := make([]string, len(items))
			for i, s := range items {
				quoted[i] = strconv.Quote(s)
			}
			return strings.Join(quoted, ", ")
		},
		"ownerOf": func(entities []*project.Entity, e *project.Entity) *Owner {
			if o, ok := OwnerUsed(entities, e); ok {
				return &o
			}
			return nil
		},
		"has": slices.Contains[[]string],
		// goElems is a []string literal without its type, for the values of
		// a map: {"a", "b"}.
		"goElems": func(items []string) string {
			quoted := make([]string, len(items))
			for i, s := range items {
				quoted[i] = strconv.Quote(s)
			}
			return "{" + strings.Join(quoted, ", ") + "}"
		},
		// manyToMany are the relations of e the API links and unlinks.
		"manyToMany": func(e *project.Entity) []*project.Relation {
			var out []*project.Relation
			for _, r := range e.Relations {
				if r.Kind == project.ManyToMany {
					out = append(out, r)
				}
			}
			return out
		},
	}
}

// UsesProvider reports whether a realm of entities signs in with provider.
func UsesProvider(entities []*project.Entity, provider string) bool {
	for _, r := range RealmsOf(entities) {
		if slices.Contains(r.Providers, provider) {
			return true
		}
	}
	return false
}

// Table is the name of one of the security tables: security_<name> in the
// public schema, or security.<name> when the postgres blueprint's schemas
// option has security.
func Table(name string, postgresOptions map[string]any) string {
	if slices.Contains(names(postgresOptions["schemas"]), "security") {
		return "security." + name
	}
	return "security_" + name
}

// Display is the column a realm's accounts are called by in their list: a
// field name, or the first text field.
func Display(e *project.Entity) string {
	for _, f := range e.Fields {
		if f.Snake == "name" {
			return f.Snake
		}
	}
	for _, f := range e.Fields {
		if !f.Implicit && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return f.Snake
		}
	}
	return ""
}

// Admins are the rules of who administers a realm's accounts: those its
// entity lets update it, but its owner (each account itself) and anyone.
func Admins(entities []*project.Entity, e *project.Entity) []string {
	out := []string{}
	for _, w := range RulesOf(entities, e, "update").Others {
		if w != "public" {
			out = append(out, w)
		}
	}
	return out
}

// ResetPageOf is a realm's page that sets a password: its path on the app's
// domain, and the server task dev serves it from in development, when that
// is another (a panel's Vite).
type ResetPageOf struct{ Path, Dev string }

// DevServer is where task dev serves a panel by default: Vite's address.
const DevServer = "http://localhost:5173"

// PanelResetPage is the page of a panel (spa) that sets a password.
const PanelResetPage = "reset-password"

// SiteResetPage is the page of a site (ssr) that sets a password, in its
// language.
func SiteResetPage(locale string) string {
	if locale == "pt_br" {
		return "nova-senha"
	}
	return "new-password"
}

// ResetPage is the page of a frontend that sets a password, as a path of
// the app's domain: /admin/reset-password; "" for a frontend on a domain
// of its own, or of a kind that has none.
func ResetPage(kind, name string, opts map[string]any) string {
	if domain, _ := opts["domain"].(string); domain != "" {
		return ""
	}
	path, _ := opts["path"].(string)
	if path == "" {
		path = "/" + name
	}
	path = strings.TrimSuffix(path, "/")
	switch kind {
	case "spa":
		return path + "/" + PanelResetPage
	case "ssr":
		locale, _ := opts["locale"].(string)
		return path + "/" + SiteResetPage(locale)
	}
	return ""
}

// ResetPages are the pages that set a password, by realm: those of the
// first frontend that signs each realm in (its auth, or the first realm),
// on the app's domain.
func ResetPages(entities []*project.Entity, frontends []*compose.Instance) map[string]ResetPageOf {
	realms := RealmsOf(entities)
	if len(realms) == 0 {
		return nil
	}
	out := map[string]ResetPageOf{}
	for _, f := range frontends {
		realm, _ := f.Options["auth"].(string)
		if realm == "" {
			realm = realms[0].Name
		}
		if _, ok := out[realm]; ok {
			continue
		}
		if page := ResetPage(f.Kind, f.Name, f.Options); page != "" {
			r := ResetPageOf{Path: page}
			if f.Kind == "spa" {
				r.Dev = DevServer
			}
			out[realm] = r
		}
	}
	return out
}
