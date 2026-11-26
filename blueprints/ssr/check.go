package ssr

import (
	"fmt"
	"github.com/patrickluzdev/esquadro/blueprints/security"
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
			"reset": security.SiteResetPage("pt_br"), "account": "conta", "confirm": "confirmar-email", "assets": "assets", "sitemap": "sitemap.xml"}
	}
	return map[string]string{"search": "search", "signin": "sign-in", "signup": "sign-up", "forgot": "forgot-password",
		"reset": security.SiteResetPage("en"), "account": "account", "confirm": "confirm-email", "assets": "assets", "sitemap": "sitemap.xml"}
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
				og := slices.ContainsFunc(Pages(p.Entities, f.Name), func(pg *Page) bool { return pg.OG })
				for _, name := range []string{"display", "body"} {
					if _, ok := b.Theme.Fonts[name]; ok && og && OGFont(b, name) == "" {
						diags.Warnf(f.Blueprint.Ref.Pos, f.Blueprint.Ref.Pos, "brand %s: the %s font has no TrueType or OpenType file (.ttf, .otf), which the Open Graph images are drawn with: they draw it in a Go font", dir, name).
							Suggestion = "name its .ttf in fonts." + name + ".files (browsers read it too)"
					}
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
		switch styles, _ := f.Options["styles"].(string); {
		case styles != "tailwind" && styles != "css":
			diags.Errorf(at("styles"), at("styles"), "styles %s is not tailwind or css", styles)
		case styles == "tailwind" && set.Get("workspace") == nil:
			diags.Errorf(f.Blueprint.Ref.Pos, f.Blueprint.Ref.Pos, "ssr %s has its styles built by Tailwind, with Node, which needs blueprint workspace", f.Name).
				Suggestion = "add blueprint workspace to app.esq, or write the styles in plain CSS: styles css"
		}
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
		if pg.OG && set.Get("images") == nil {
			diags.Errorf(where, where, "og_image of %s on %s draws each record's Open Graph image on the server, which needs blueprint images (and blueprint storage, where it keeps them)", e.Raw, name).
				Suggestion = "add blueprint storage and blueprint images to app.esq"
		}
	}
	diags = append(diags, checkAddresses(p, name)...)
	// og_image is of a record's page: an entity without pages has none.
	for _, e := range p.Entities {
		a := e.Ann[name]
		if og, _ := a["og_image"].(bool); og {
			if page, _ := a["page"].(bool); !page {
				diags.Errorf(e.Pos, e.Pos, "og_image of %s is the image of its records' pages on %s, which it has not: @%s(page, og_image)", e.Raw, name, name)
			}
		}
	}
	return diags
}

// checkAddresses checks what a site's addresses and structured data are
// made of: key, seller, browse and available.
func checkAddresses(p *project.Project, name string) diag.List {
	var diags diag.List
	text := []project.Type{project.TypeString, project.TypeText}
	pages := map[*project.Entity]*Page{}
	for _, pg := range Pages(p.Entities, name) {
		pages[pg.Entity] = pg
	}
	for _, e := range p.Entities {
		a, pg := e.Ann[name], pages[e]
		if k, ok := a["key"]; ok {
			f := fieldNamed(e, k)
			switch {
			case pg == nil:
				diags.Errorf(e.Pos, e.Pos, "key of %s ends the address of its records' pages on %s, which it has not: @%s(page, key %v)", e.Raw, name, name, k)
			case f == nil:
				diags.Errorf(e.Pos, e.Pos, "key %v of @%s is not a field of %s", k, name, e.Raw)
			case !f.Unique || !slices.Contains(text, f.Type):
				diags.Errorf(f.Pos, f.Pos, "key of @%s: %s.%s must be a unique string, for a record's page to be found by it", name, e.Raw, f.Raw).
					Suggestion = f.Raw + " string unique"
			}
		}
		if s, ok := a["seller"]; ok {
			f := fieldNamed(e, s)
			switch {
			case pg == nil || pg.Schema != "product":
				diags.Errorf(e.Pos, e.Pos, "seller of %s is of a product's structured data: @%s(page, schema product, seller %v)", e.Raw, name, s)
			case f == nil:
				diags.Errorf(e.Pos, e.Pos, "seller %v of @%s is not a field of %s", s, name, e.Raw)
			case f.Enum == nil && !slices.Contains(text, f.Type):
				diags.Errorf(f.Pos, f.Pos, "seller of @%s: %s.%s is %s; who sells a product is a text or an enum", name, e.Raw, f.Raw, f.Type)
			}
		}
		available := 0
		for _, f := range e.Fields {
			fa := f.Ann[name]
			if values, ok := fa["available"]; ok {
				available++
				switch {
				case pg == nil || pg.Schema != "product":
					diags.Errorf(f.Pos, f.Pos, "available of %s.%s is of a product's structured data: @%s(page, schema product) on %s", e.Raw, f.Raw, name, e.Raw)
				case f.Enum == nil:
					diags.Errorf(f.Pos, f.Pos, "available of @%s: %s.%s is %s; a product is in stock by the values of an enum", name, e.Raw, f.Raw, f.Type)
				case len(ui.Strings(values)) == 0:
					diags.Errorf(f.Pos, f.Pos, "available of %s.%s names no value of %s: available [%s]", e.Raw, f.Raw, f.Enum.Raw, f.Enum.Values[0].Raw)
				case available > 1:
					diags.Errorf(f.Pos, f.Pos, "available of %s.%s: %s is in stock by one field at most", e.Raw, f.Raw, e.Raw)
				default:
					for _, v := range ui.Strings(values) {
						if f.Enum.Value(v) == nil {
							diags.Errorf(f.Pos, f.Pos, "available of %s.%s: %s is not a value of %s", e.Raw, f.Raw, v, f.Enum.Raw)
						}
					}
				}
			}
			diags = append(diags, checkBrowse(name, e, pg, f.Ann[name], f.Pos, e.Raw+"."+f.Raw, f, nil)...)
		}
		for _, r := range e.Relations {
			diags = append(diags, checkBrowse(name, e, pg, r.Ann[name], r.Pos, e.Raw+"."+r.Raw, nil, r)...)
		}
		if pg == nil {
			continue
		}
		bare, segments := 0, map[string]bool{}
		for _, b := range pg.Browses {
			if b.Segment == "" {
				bare++
				if bare > 1 {
					diags.Errorf(e.Pos, e.Pos, "%s has more than one browse page with no path on %s: give each but one a path, the segment of its address", e.Raw, name)
				}
				continue
			}
			if segments[b.Segment] {
				diags.Errorf(e.Pos, e.Pos, "%s has two browse pages at the same path on %s: %s", e.Raw, name, b.Segment)
			}
			segments[b.Segment] = true
		}
	}
	return diags
}

// checkBrowse checks browse and path of a field or a relation (rel): a
// filter of a list with pages, of an enum or a related record with a
// title; its path, a segment of an address.
func checkBrowse(name string, e *project.Entity, pg *Page, a map[string]any, pos diag.Pos, what string, fld *project.Field, rel *project.Relation) diag.List {
	var diags diag.List
	on, _ := a["browse"].(bool)
	path, hasPath := a["path"].(string)
	if hasPath && !on {
		diags.Errorf(pos, pos, "path of %s is the segment of a browse page's address: @%s(browse, path %q)", what, name, path)
	}
	if !on {
		return diags
	}
	switch {
	case pg == nil:
		diags.Errorf(pos, pos, "browse of %s is a page of %s's list on %s, which it has not: @%s(page) on %s", what, e.Raw, name, name, e.Raw)
	case fld != nil && fld.Enum == nil:
		diags.Errorf(pos, pos, "browse of %s: a browse page is of an enum's value or a related record", what)
	case rel != nil && !rel.OwnsForeignKey():
		diags.Errorf(pos, pos, "browse of %s: a browse page is of the one record a relation points at (written > %s)", what, rel.Target.Raw)
	case !slices.ContainsFunc(pg.Filters, func(f Filter) bool {
		return (rel != nil && f.Relation == rel) || (fld != nil && f.Kind == "in" && f.Field == fld.Pascal)
	}):
		diags.Errorf(pos, pos, "browse of %s: its page lists %s by it, which needs @store(filter)", what, e.Plural.Raw)
	case rel != nil && TitleOf(rel.Target, name) == nil:
		diags.Errorf(pos, pos, "browse of %s: %s has no text field its pages' addresses are made of", what, rel.Target.Raw).
			Suggestion = "add one, or name it with @" + name + "(title ...) on " + rel.Target.Raw
	}
	if hasPath && on && (path == "" || Slug(path) != path) {
		diags.Errorf(pos, pos, "path %q of %s is no segment of an address: lowercase letters, digits and hyphens, e.g. %q", path, what, Slug(path))
	}
	return diags
}

// Slug is a title as an address, as the site writes it: lowercase
// letters and digits, joined by hyphens.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	return b.String()
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
