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
// widgetKinds are the widgets a report may be on a dashboard.
var widgetKinds = []string{"stat", "area", "line", "bar", "combo", "donut", "progress", "goal", "top", "table", "radar", "timeline"}

// widthNames are how wide a widget's card may be.
var widthNames = []string{"third", "half", "two_thirds", "full"}

func widgets(p *project.Project, instance string) diag.List {
	var diags diag.List
	for _, r := range p.Reports {
		a := r.Ann[instance]
		kind, _ := a["widget"].(string)
		if kind == "" {
			continue
		}
		if !slices.Contains(widgetKinds, kind) {
			diags.Errorf(r.Pos, r.Pos, "widget %s of %s is not %s", kind, r.Raw, strings.Join(widgetKinds, ", "))
			continue
		}
		column := func(name string) *project.Field {
			for _, f := range r.Columns {
				if f.Raw == name {
					return f
				}
			}
			return nil
		}
		ys := strs(a["y"])
		for _, key := range []string{"x", "series", "target", "image", "subtitle", "delta"} {
			if col, _ := a[key].(string); col != "" && column(col) == nil {
				diags.Errorf(r.Pos, r.Pos, "%s %s of %s's widget is not one of its columns", key, col, r.Raw)
			}
		}
		for _, y := range ys {
			if column(y) == nil {
				diags.Errorf(r.Pos, r.Pos, "y %s of %s's widget is not one of its columns", y, r.Raw)
			}
		}
		if kind != "table" && len(ys) == 0 {
			diags.Errorf(r.Pos, r.Pos, "%s's widget %s needs y, the column of its numbers", r.Raw, kind)
		}
		needsX := kind != "stat" && kind != "goal" && kind != "table"
		if needsX && a["x"] == nil {
			diags.Errorf(r.Pos, r.Pos, "%s's widget %s needs x, the column of its names", r.Raw, kind)
		}
		if kind == "goal" && a["target"] == nil {
			diags.Errorf(r.Pos, r.Pos, "%s's widget goal needs target, the column of its goal", r.Raw)
		}
		if kind == "combo" && len(ys) != 2 {
			diags.Errorf(r.Pos, r.Pos, "%s's widget combo needs two y columns, [bars, line]", r.Raw)
		}
		if a["series"] != nil && len(ys) > 1 {
			diags.Errorf(r.Pos, r.Pos, "%s's widget has series and several y columns: one or the other", r.Raw)
		}
		for _, key := range []string{"since", "until"} {
			if param, _ := a[key].(string); param != "" && !slices.ContainsFunc(r.Params, func(f *project.Field) bool { return f.Raw == param }) {
				diags.Errorf(r.Pos, r.Pos, "%s %s of %s's widget is not one of its parameters", key, param, r.Raw)
			}
		}
		if a["until"] != nil && a["since"] == nil {
			diags.Errorf(r.Pos, r.Pos, "%s's widget has until without since: the period's start comes first", r.Raw)
		}
		if t, _ := a["total"].(string); t != "" && !slices.Contains([]string{"sum", "last", "average", "max"}, t) {
			diags.Errorf(r.Pos, r.Pos, "total %s of %s's widget is not sum, last, average or max", t, r.Raw)
		}
		if n, ok := a["rows"].(int64); ok && n < 1 {
			diags.Errorf(r.Pos, r.Pos, "rows %d of %s's widget is less than one", n, r.Raw)
		}
		if w, _ := a["width"].(string); w != "" && !slices.Contains(widthNames, w) {
			diags.Errorf(r.Pos, r.Pos, "width %s of %s's widget is not %s", w, r.Raw, strings.Join(widthNames, ", "))
		}
		for _, f := range r.Columns {
			if format, _ := f.Ann[instance]["format"].(string); format != "" && format != "percent" && format != "duration" {
				diags.Errorf(f.Pos, f.Pos, "format %s of %s is not percent or duration", format, f.Raw)
			}
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
