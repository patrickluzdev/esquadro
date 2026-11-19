package ui

import (
	"fmt"
	"go/token"
	"regexp"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Locales are the languages a frontend's words, dates and money come in.
var Locales = []string{"en", "pt_br"}

// Frontend is a frontend of the app: an instance of a blueprint that
// requires ui (spa admin, ssr site).
type Frontend struct {
	Blueprint *blueprint.Blueprint
	// Name is the instance's (admin); Kind its blueprint's (spa).
	Name, Kind string
	Options    map[string]any
	// Realm is the realm that signs in to it, once checked.
	Realm *security.Realm
}

// Frontends are the app's frontends, in the order the app declares them.
func Frontends(set *blueprint.Set) []*Frontend {
	var out []*Frontend
	for _, b := range set.Blueprints {
		if b.Kind == "" || !slices.ContainsFunc(b.Manifest.Requires, func(d blueprint.Dep) bool { return d.Name == "ui" }) {
			continue
		}
		out = append(out, &Frontend{Blueprint: b, Name: b.Manifest.Name, Kind: b.Kind, Options: set.Options[b.Manifest.Name]})
	}
	return out
}

// Domain is the domain a frontend answers on, "" for the app's.
func (f *Frontend) Domain() string { s, _ := f.Options["domain"].(string); return s }

// Path is where a frontend is on its domain: / on a domain of its own,
// its path (/<name> by default) on the app's.
func (f *Frontend) Path() string {
	if f.Domain() != "" {
		return "/"
	}
	if p, _ := f.Options["path"].(string); p != "" {
		return p
	}
	return "/" + f.Name
}

// at is where an option of the frontend is written, or its blueprint line.
func (f *Frontend) at(key string) diag.Pos {
	for _, o := range f.Blueprint.Ref.Options {
		if o.Key.Name == key {
			return o.Pos
		}
	}
	return f.Blueprint.Ref.Pos
}

var (
	domainName = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)
	pathName   = regexp.MustCompile(`^(/[a-z0-9][a-z0-9_-]*)+$`)
)

// CheckOptions checks the options every frontend has, the same way for
// each: its name, domain, path, realm, roles, locale, serve and colors. It
// sets f.Realm; false when the frontend cannot be checked further.
func CheckOptions(p *project.Project, f *Frontend) (diag.List, bool) {
	var diags diag.List
	ref, name, opts := f.Blueprint.Ref, f.Name, f.Options
	if !token.IsIdentifier(name) || token.IsKeyword(name) || name != strings.ToLower(name) {
		diags.Errorf(ref.InstanceSpan.Pos, ref.InstanceSpan.End, "%s cannot name a frontend: it becomes a Go package and a folder, so lowercase letters and digits", name).
			Suggestion = fmt.Sprintf("e.g. blueprint %s admin", f.Kind)
	}
	if d := f.Domain(); d != "" && !domainName.MatchString(d) {
		diags.Errorf(f.at("domain"), f.at("domain"), "domain %s is not a domain name: shop.com, admin.shop.com", d)
	}
	if path, _ := opts["path"].(string); path != "" {
		switch {
		case f.Domain() != "":
			diags.Errorf(f.at("path"), f.at("path"), "path %s goes with no domain: on a domain of its own, %s %s is at its root", path, f.Kind, name)
		case !pathName.MatchString(path):
			diags.Errorf(f.at("path"), f.at("path"), "path %s is not a path: /admin, /portal", path)
		}
	}
	realms := security.RealmsOf(p.Entities)
	if len(realms) == 0 {
		diags.Errorf(ref.Pos, ref.Pos, "%s %s signs people in, and no entity is marked @security(principal)", f.Kind, name).
			Suggestion = "mark the entity of the people who use it @security(principal)"
		return diags, false
	}
	f.Realm = realms[0]
	if auth, _ := opts["auth"].(string); auth != "" {
		f.Realm = nil
		for _, r := range realms {
			if r.Name == auth {
				f.Realm = r
			}
		}
		if f.Realm == nil {
			diags.Errorf(f.at("auth"), f.at("auth"), "auth %s is not a realm: %s", auth, realmNames(realms))
			return diags, false
		}
	}
	for _, role := range Strings(opts["roles"]) {
		if !slices.Contains(f.Realm.Roles, role) {
			diags.Errorf(f.at("roles"), f.at("roles"), "roles: %s is not a role of %s, whose roles are %s", role, f.Realm.Name, strings.Join(f.Realm.Roles, ", "))
		}
	}
	if serve, _ := opts["serve"].(string); serve != "app" {
		diags.Errorf(f.at("serve"), f.at("serve"), "serve %s is not one esquadro generates yet: app is (the app's binary serves it)", serve)
	}
	if locale, _ := opts["locale"].(string); !slices.Contains(Locales, locale) {
		diags.Errorf(f.at("locale"), f.at("locale"), "locale %s is not %s: choose en, and add the language's words to the frontend's own", locale, strings.Join(Locales, " or "))
	}
	for _, o := range []string{"primary", "primary_dark"} {
		if v, _ := opts[o].(string); ColorError(o, v) != "" {
			diags.Errorf(f.at(o), f.at(o), "%s", ColorError(o, v))
		}
	}
	return diags, true
}

// Check checks the frontends together: no two where one answers, and none
// where the app's own routes are.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	reserved := []string{"/api", "/healthz", "/readyz", "/docs", "/.well-known"}
	if sec := set.Options["security"]; sec != nil {
		if path, _ := sec["path"].(string); path != "" {
			reserved = append(reserved, path)
		}
	}
	seen := map[string]*Frontend{}
	for _, f := range Frontends(set) {
		where := f.Domain() + f.Path()
		if other := seen[where]; other != nil {
			diags.Errorf(f.Blueprint.Ref.Pos, f.Blueprint.Ref.Pos, "%s %s answers where %s %s does (%s): give one a domain or a path of its own", f.Kind, f.Name, other.Kind, other.Name, where)
		}
		seen[where] = f
		if f.Domain() != "" {
			continue
		}
		for _, r := range reserved {
			if f.Path() == r || strings.HasPrefix(f.Path(), r+"/") {
				diags.Errorf(f.at("path"), f.at("path"), "%s %s would be at %s, where the app's own routes are: choose another path", f.Kind, f.Name, f.Path())
			}
		}
	}
	return diags
}

func realmNames(realms []*security.Realm) string {
	names := make([]string, len(realms))
	for i, r := range realms {
		names[i] = r.Name
	}
	return "the realms are " + strings.Join(names, ", ")
}

// Strings is an option's list of names or strings.
func Strings(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
