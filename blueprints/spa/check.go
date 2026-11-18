package spa

import (
	"go/token"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check checks each instance of spa: its name (a Go package and a folder),
// the realm that signs in to it and its roles, and where it is served.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	realms := security.RealmsOf(p.Entities)
	for _, b := range set.Blueprints {
		if b.Kind != "spa" {
			continue
		}
		ref, name, opts := b.Ref, b.Manifest.Name, set.Options[b.Manifest.Name]
		at := func(key string) diag.Pos {
			for _, o := range ref.Options {
				if o.Key.Name == key {
					return o.Pos
				}
			}
			return ref.Pos
		}
		if !token.IsIdentifier(name) || token.IsKeyword(name) || name != strings.ToLower(name) {
			diags.Errorf(ref.InstanceSpan.Pos, ref.InstanceSpan.End, "%s cannot name a frontend: it becomes a Go package and a folder, so lowercase letters and digits", name).
				Suggestion = "e.g. blueprint spa admin"
		}
		if len(realms) == 0 {
			diags.Errorf(ref.Pos, ref.Pos, "spa %s signs people in, and no entity is marked @security(principal)", name).
				Suggestion = "mark the entity of the people who use it @security(principal)"
			continue
		}
		realm := realms[0]
		if auth, _ := opts["auth"].(string); auth != "" {
			realm = nil
			for _, r := range realms {
				if r.Name == auth {
					realm = r
				}
			}
			if realm == nil {
				diags.Errorf(at("auth"), at("auth"), "auth %s is not a realm: %s", auth, realmNames(realms))
				continue
			}
		}
		for _, role := range strs(opts["roles"]) {
			if !slices.Contains(realm.Roles, role) {
				diags.Errorf(at("roles"), at("roles"), "roles: %s is not a role of %s, whose roles are %s", role, realm.Name, strings.Join(realm.Roles, ", "))
			}
		}
		if serve, _ := opts["serve"].(string); serve != "app" {
			diags.Errorf(at("serve"), at("serve"), "serve %s is not one esquadro generates yet: app is (the app's binary serves it)", serve)
		}
		if locale, _ := opts["locale"].(string); locale != "en" && locale != "pt_br" {
			diags.Errorf(at("locale"), at("locale"), "locale %s is not en or pt_br: add the language to src/messages.ts, and choose en here", locale)
		}
		if set.Get("workspace") == nil {
			diags.Errorf(ref.Pos, ref.Pos, "spa %s lives in the workspace: add blueprint workspace", name)
		}
	}
	return diags
}

func realmNames(realms []*security.Realm) string {
	var out []string
	for _, r := range realms {
		out = append(out, r.Name)
	}
	return strings.Join(out, ", ")
}

func strs(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
