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
		"devServers":    DevServers,
		"redirectURLs":  RedirectURLs,
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
// domain, or its address on its frontend's own.
type ResetPageOf struct{ Page string }

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

// ResetPage is the page of a frontend that sets a password: a path of the
// app's domain, /admin/reset-password, or an address on the frontend's own,
// https://admin.shop.com/reset-password; "" for a frontend of a kind that
// has none.
func ResetPage(kind, name string, opts map[string]any) string {
	path, _ := opts["path"].(string)
	if path == "" {
		path = "/" + name
	}
	path = strings.TrimSuffix(path, "/")
	if domain, _ := opts["domain"].(string); domain != "" {
		path = "https://" + domain
	}
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
// first frontend that signs each realm in (its auth, or the first realm).
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
			out[realm] = ResetPageOf{Page: page}
		}
	}
	return out
}

// DevServerOf is where the links to a frontend open in development: those
// under At, a path of the app's domain (/admin) or the frontend's own
// domain (admin.shop.com), open at URL (task dev's, http://localhost:5173),
// or, with Host, on the app's address in development with that host
// instead (a site's alias, site.localhost: http://site.localhost:8080).
type DevServerOf struct{ At, URL, Host string }

// DevServers are where the app's frontends are in development, by where
// they are otherwise: a panel where task dev serves it, on the app's domain
// or its own; a site on a domain of its own, on its alias on the app's
// port. A site on the app's domain is where the app is.
func DevServers(frontends []*compose.Instance) []DevServerOf {
	var out []DevServerOf
	seen := map[string]bool{}
	for _, f := range frontends {
		domain, _ := f.Options["domain"].(string)
		var d DevServerOf
		switch {
		case f.Kind == "spa" && domain != "":
			d = DevServerOf{At: domain, URL: DevServer}
		case f.Kind == "spa":
			path, _ := f.Options["path"].(string)
			if path == "" {
				path = "/" + f.Name
			}
			d = DevServerOf{At: strings.TrimSuffix(path, "/"), URL: DevServer}
		case f.Kind == "ssr" && domain != "":
			d = DevServerOf{At: domain, Host: f.Name + ".localhost"}
		default:
			continue
		}
		if !seen[d.At] {
			seen[d.At] = true
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b DevServerOf) int { return strings.Compare(a.At, b.At) })
	return out
}

// RedirectURLs are where, besides the app's own paths, people may be sent:
// the security blueprint's redirect_urls, and the domains of the frontends
// that have one of their own, over HTTPS.
func RedirectURLs(listed []string, frontends []*compose.Instance) []string {
	out := slices.Clone(listed)
	for _, f := range frontends {
		if domain, _ := f.Options["domain"].(string); domain != "" && !slices.Contains(out, "https://"+domain) {
			out = append(out, "https://"+domain)
		}
	}
	return out
}
