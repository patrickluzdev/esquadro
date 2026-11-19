package ssr

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Schemas are the structured data a page may have.
var Schemas = []string{"product", "article", "organization"}

// SystemPaths are the addresses of a site's own pages, in its language:
// search, sign in, sign up, the forgotten password, a new one, the
// account, an email confirmed, the static files and the sitemap.
func SystemPaths(locale string) map[string]string {
	if locale == "pt_br" {
		return map[string]string{"search": "busca", "signin": "entrar", "signup": "cadastro", "forgot": "esqueci-a-senha",
			"reset": "nova-senha", "account": "conta", "confirm": "confirmar-email", "assets": "assets", "sitemap": "sitemap.xml"}
	}
	return map[string]string{"search": "search", "signin": "sign-in", "signup": "sign-up", "forgot": "forgot-password",
		"reset": "new-password", "account": "account", "confirm": "confirm-email", "assets": "assets", "sitemap": "sitemap.xml"}
}

// Check checks each site: its options, as every frontend's, its engine and
// address, and the pages of its entities.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	for _, f := range ui.Frontends(set) {
		if f.Kind != "ssr" {
			continue
		}
		d, ok := ui.CheckOptions(p, f)
		diags = append(diags, d...)
		if !ok {
			continue
		}
		if dir, _ := f.Options["brand"].(string); dir != "" {
			b, err := ui.BrandOf(p.App.Pos.File, f.Options)
			switch {
			case err != nil:
				diags.Errorf(f.Blueprint.Ref.Pos, f.Blueprint.Ref.Pos, "%v", err)
			case b != nil:
				for _, problem := range b.Problems() {
					diags.Errorf(f.Blueprint.Ref.Pos, f.Blueprint.Ref.Pos, "brand %s: %s", dir, problem)
				}
			}
		}
		at := func(key string) diag.Pos {
			for _, o := range f.Blueprint.Ref.Options {
				if o.Key.Name == key {
					return o.Pos
				}
			}
			return f.Blueprint.Ref.Pos
		}
		diags = append(diags, checkRedirects(p, f.Name)...)
		if engine, _ := f.Options["engine"].(string); engine != "go" {
			diags.Errorf(at("engine"), at("engine"), "engine %s is not one esquadro generates yet: go is (the app's binary renders the site)", engine)
		}
		if u, _ := f.Options["url"].(string); u != "" {
			if parsed, err := url.Parse(u); err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || strings.TrimSuffix(parsed.Path, "/") != "" {
				diags.Errorf(at("url"), at("url"), "url %s is not a site's address: https://shop.com", u)
			}
		}
		if n, _ := f.Options["page_size"].(int64); n < 1 || n > 100 {
			diags.Errorf(at("page_size"), at("page_size"), "page_size %d is not between 1 and 100", n)
		}
		locale, _ := f.Options["locale"].(string)
		diags = append(diags, checkPages(p, set, f, SystemPaths(locale))...)
		if locale != "en" {
			diags = append(diags, unlabeled(p, f.Name)...)
		}
	}
	return diags
}

func checkPages(p *project.Project, set *blueprint.Set, f *ui.Frontend, system map[string]string) diag.List {
	var diags diag.List
	name := f.Name
	paths := map[string]*project.Entity{}
	for _, pg := range Pages(p.Entities, name) {
		e, a := pg.Entity, pg.Entity.Ann[name]
		where := e.Pos
		if other := paths[pg.Path]; other != nil {
			diags.Errorf(where, where, "%s and %s have the same path on %s: %s", other.Raw, e.Raw, name, pg.Path)
		}
		paths[pg.Path] = e
		for key, p := range system {
			if pg.Path == p {
				diags.Errorf(where, where, "path %s of %s is where %s %s has its %s page: choose another", pg.Path, e.Raw, f.Kind, name, key)
			}
		}
		for _, key := range []string{"title", "description", "image"} {
			if s, _ := a[key].(string); s != "" && fieldNamed(e, s) == nil {
				diags.Errorf(where, where, "%s %s of @%s is not a field of %s", key, s, name, e.Raw)
			}
		}
		if s, _ := a["image"].(string); s != "" {
			if fld := fieldNamed(e, s); fld != nil && fld.Type != project.TypeImage {
				diags.Errorf(where, where, "image %s of @%s is not an image field of %s", s, name, e.Raw)
			}
		}
		if pg.Title == nil {
			diags.Errorf(where, where, "%s has no text field to call its records by on %s: add one, or name it with title", e.Raw, name)
		}
		if pg.Schema != "" && !slices.Contains(Schemas, pg.Schema) {
			diags.Errorf(where, where, "schema %s of %s is not %s", pg.Schema, e.Raw, strings.Join(Schemas, ", "))
		}
		if pg.Sort != "" && !slices.Contains(pg.Sorts, pg.Sort) {
			diags.Errorf(where, where, "sort %s of %s is not an order its list takes: %s (mark a field @store(sort))", pg.Sort, e.Raw, strings.Join(pg.Sorts, ", "))
		}
		for _, rel := range ui.Strings(a["relations"]) {
			r := relationNamed(e, rel)
			switch {
			case r == nil:
				diags.Errorf(where, where, "relations: %s is not a relation of %s", rel, e.Raw)
			case r.Kind != project.OneToMany || r.Inverse == nil:
				diags.Errorf(where, where, "relations: %s of %s is not a list of records pointing at it (written < %s)", rel, e.Raw, r.Target.Raw)
			case !filters(r.Target, r.Inverse.Pascal+"ID"):
				diags.Errorf(where, where, "relations: %s's page lists its %s by %s.%s, which needs @store(filter)", e.Raw, rel, r.Target.Raw, r.Inverse.Raw)
			}
		}
		if !readable(e, f) {
			diags.Errorf(where, where, "%s has pages on %s, which nobody there may read: its @security(read ...) has neither public nor %s", e.Raw, name, f.Realm.Name).
				Suggestion = "e.g. @security(read public, ...)"
		}
		if pg.Image != nil && set.Get("storage") == nil {
			diags.Errorf(where, where, "%s's pages show its image, which needs blueprint storage", e.Raw)
		}
	}
	return diags
}

// filters reports whether e's list filters by a field (CategoryID).
func filters(e *project.Entity, field string) bool {
	for _, f := range store.ListFilters(e) {
		if f.Field == field {
			return true
		}
	}
	return false
}

// readable reports whether someone on the site may read e: everyone, or
// who signs in to its realm.
func readable(e *project.Entity, f *ui.Frontend) bool {
	for _, who := range ui.Strings(e.Ann["security"]["read"]) {
		realm, _, _ := strings.Cut(who, ".")
		if who == "public" || realm == f.Realm.Name {
			return true
		}
	}
	return false
}

// unlabeled warns of what a site shows with no label of its own, whose
// locale is not English: it would show names in English.
func unlabeled(p *project.Project, instance string) diag.List {
	var diags diag.List
	warn := func(pos diag.Pos, what, shows string) {
		diags.Warnf(pos, pos, "%s has no label in %s, whose locale is not en: it shows %q", what, instance, shows).
			Suggestion = `@ui(label "...")`
	}
	warned := map[*project.Enum]bool{}
	for _, pg := range Pages(p.Entities, instance) {
		e := pg.Entity
		if !ui.Labeled(e.Ann, instance) {
			warn(e.Pos, e.Raw, ui.Humanize(e.Plural.Snake))
		}
		for _, fld := range pg.Fields {
			if !ui.Labeled(fld.Ann, instance) {
				warn(fld.Pos, fmt.Sprintf("%s.%s", e.Raw, fld.Raw), ui.Humanize(fld.Snake))
			}
			if fld.Enum != nil && !warned[fld.Enum] {
				warned[fld.Enum] = true
				for _, v := range fld.Enum.Values {
					if !ui.Labeled(v.Ann, instance) {
						warn(v.Pos, fmt.Sprintf("%s.%s", fld.Enum.Raw, v.Raw), v.Raw)
					}
				}
			}
		}
	}
	return diags
}
