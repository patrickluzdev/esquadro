package spa

import (
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/rest"
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
		if sidebar, _ := opts["sidebar"].(string); sidebar != "icon" && sidebar != "inset" && sidebar != "dashboard" {
			diags.Errorf(at("sidebar"), at("sidebar"), "sidebar %s is not icon, inset or dashboard", sidebar)
		}
		for _, o := range []string{"primary", "primary_dark"} {
			if v, _ := opts[o].(string); ColorError(o, v) != "" {
				diags.Errorf(at(o), at(o), "%s", ColorError(o, v))
			}
		}
		if filters, _ := opts["filters"].(string); filters != "drawer" && filters != "dialog" && filters != "bar" {
			diags.Errorf(at("filters"), at("filters"), "filters %s is not drawer, dialog or bar", filters)
		}
		if locale, _ := opts["locale"].(string); locale != "en" {
			diags = append(diags, unlabeled(p, name)...)
		}
		diags = append(diags, widgets(p, name)...)
		if idle, _ := opts["idle_minutes"].(int64); idle != 0 && idle < 2 {
			diags.Errorf(at("idle_minutes"), at("idle_minutes"), "idle_minutes %d is too short: at least 2, a minute of warning and one of use, or 0 for never", idle)
		}
		if set.Get("workspace") == nil {
			diags.Errorf(ref.Pos, ref.Pos, "spa %s lives in the workspace: add blueprint workspace", name)
		}
	}
	return diags
}

// unlabeled warns of the fields of a frontend's resources that have no
// label of its own: they would show their names, in English.
func unlabeled(p *project.Project, instance string) diag.List {
	var diags diag.List
	warned := map[*project.Enum]bool{}
	labeled := func(ann project.Ann) bool {
		label, _ := ann[instance]["label"].(string)
		return label != ""
	}
	for _, r := range p.Reports {
		if widget, _ := r.Ann[instance]["widget"].(string); widget == "" {
			continue
		}
		for _, c := range r.Columns {
			if !labeled(c.Ann) {
				diags.Warnf(c.Pos, c.Pos, "%s.%s has no label in %s, whose locale is not en: its widget shows %q", r.Raw, c.Raw, instance, Humanize(c.Snake)).
					Suggestion = fmt.Sprintf(`@%s(label "...")`, instance)
			}
		}
	}
	for _, e := range p.Entities {
		if resource, _ := e.Ann[instance]["resource"].(bool); !resource {
			continue
		}
		for _, f := range rest.InRequest(e) {
			if !labeled(f.Ann) {
				diags.Warnf(f.Pos, f.Pos, "%s.%s has no label in %s, whose locale is not en: it shows %q", e.Raw, f.Raw, instance, Humanize(f.Snake)).
					Suggestion = fmt.Sprintf(`@%s(label "...")`, instance)
			}
		}
		for _, f := range rest.InRequest(e) {
			if f.Enum == nil || warned[f.Enum] {
				continue
			}
			warned[f.Enum] = true
			for _, v := range f.Enum.Values {
				if !labeled(v.Ann) {
					diags.Warnf(v.Pos, v.Pos, "%s.%s has no label in %s, whose locale is not en: it shows %q", f.Enum.Raw, v.Raw, instance, v.Raw).
						Suggestion = fmt.Sprintf(`@%s(label "...")`, instance)
				}
			}
		}
		for _, r := range rest.Keys(e) {
			if !labeled(r.Ann) {
				diags.Warnf(r.Pos, r.Pos, "%s.%s has no label in %s, whose locale is not en: it shows %q", e.Raw, r.Raw, instance, Humanize(r.Snake)).
					Suggestion = fmt.Sprintf(`@%s(label "...")`, instance)
			}
		}
	}
	return diags
}

// widgets checks the reports on an instance's dashboard: their kind, and
// the columns and parameter they name.
func widgets(p *project.Project, instance string) diag.List {
	var diags diag.List
	for _, r := range p.Reports {
		a := r.Ann[instance]
		kind, _ := a["widget"].(string)
		if kind == "" {
			continue
		}
		if !slices.Contains([]string{"stat", "bar", "line", "table"}, kind) {
			diags.Errorf(r.Pos, r.Pos, "widget %s of %s is not stat, bar, line or table", kind, r.Raw)
		}
		has := func(fields []*project.Field, name string) bool {
			return slices.ContainsFunc(fields, func(f *project.Field) bool { return f.Raw == name })
		}
		for _, key := range []string{"x", "y"} {
			if col, _ := a[key].(string); col != "" && !has(r.Columns, col) {
				diags.Errorf(r.Pos, r.Pos, "%s %s of %s's widget is not one of its columns", key, col, r.Raw)
			}
		}
		if (kind == "stat" || kind == "bar" || kind == "line") && a["y"] == nil {
			diags.Errorf(r.Pos, r.Pos, "%s's widget %s needs y, the column of its numbers", r.Raw, kind)
		}
		if (kind == "bar" || kind == "line") && a["x"] == nil {
			diags.Errorf(r.Pos, r.Pos, "%s's widget %s needs x, the column of its names", r.Raw, kind)
		}
		if since, _ := a["since"].(string); since != "" && !has(r.Params, since) {
			diags.Errorf(r.Pos, r.Pos, "since %s of %s's widget is not one of its parameters", since, r.Raw)
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
