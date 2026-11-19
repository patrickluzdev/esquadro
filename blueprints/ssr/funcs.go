// Package ssr is the ssr blueprint's code: what its templates know of the
// pages each entity has, its checks and its hook, which writes the Go of
// the site's templ files.
package ssr

import (
	"slices"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the ssr blueprint's template functions.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"sitePages":     Pages,
		"siteRedirects": Redirects,
		"siteField": func(instance, suffix string) string {
			return project.FieldName(strings.ToUpper(instance) + "_" + suffix)
		},
		"siteSample":             store.Sample,
		"siteCanSample":          store.CanSample,
		"canRedirectSample":      canRedirectSample,
		"redirectSampleImports":  redirectSampleImports,
		"redirectSamplePackages": redirectSamplePackages,
		"sitePage":               PageOf,
		"siteModules":            Modules,
		"sitePageModules":        PageModules,
		"siteLabel":              func(a project.Ann, instance, fallback string) string { return ui.Label(a, instance, fallback) },
		"humanize":               ui.Humanize,
		"uiTheme":                ui.ThemeOf,
		"uiWords":                ui.Words,
		"siteWords":              Words,
		"realmOf":                realmOf,
		"lower":                  strings.ToLower,
		"sitePaths":              SystemPaths,
		"add1":                   func(n int) int { return n + 1 },
		"siteValue":              Value,
		"siteShown":              Shown,
		"sitePlain":              Plain,
		"siteImages": func(pages []*Page) bool {
			return slices.ContainsFunc(pages, func(p *Page) bool { return p.Image != nil })
		},
		"siteSearch": func(pages []*Page) bool {
			return slices.ContainsFunc(pages, func(p *Page) bool { return p.Search })
		},
		"siteMoney": func(pages []*Page) bool {
			return slices.ContainsFunc(pages, func(p *Page) bool {
				return slices.ContainsFunc(p.Fields, func(f *project.Field) bool { return f.Type == project.TypeMoney })
			})
		},
		"siteEnums": func(pages []*Page) []*project.Enum {
			var out []*project.Enum
			for _, p := range pages {
				for _, f := range p.Fields {
					if f.Enum != nil && !slices.Contains(out, f.Enum) {
						out = append(out, f.Enum)
					}
				}
			}
			return out
		},
	}
}

// Page is an entity with pages on a site: its list and a page per record.
type Page struct {
	Entity *project.Entity
	// Path is its part of the address: ofertas.
	Path string
	// Title, Description and Image are the fields a record is called by,
	// described by and pictured by; Description and Image may be nil.
	Title, Description, Image *project.Field
	// Schema is its structured data: product, article, organization, or
	// none.
	Schema string
	// Sort is its list's first order.
	Sort string
	// Sorts are the orders its list offers; Search, whether it searches.
	Sorts  []string
	Search bool
	// Filters are its list's filters people choose: enums, yes or no,
	// related records.
	Filters []Filter
	// Lists are the related records its page lists.
	Lists []*List
	// Menu puts it in the site's menu.
	Menu bool
	// Fields are those the site shows, in order: none hidden, no files.
	Fields []*project.Field
	// Keys are its relations to one record the site shows.
	Keys []*project.Relation
}

// Filter is a filter of a list on a site.
type Filter struct {
	// Param is its name in the address: status, category_id.
	Param string
	// Kind is in (enum values), bool or id (a related record).
	Kind string
	// Field is the domain filter's field: Status, CategoryID.
	Field string
	// Enum is the enum of an in filter; Relation, the relation of an id
	// one.
	Enum     *project.Enum
	Relation *project.Relation
	// Source is the field or relation it filters, for its label.
	Ann project.Ann
	// Name is what it is called by default.
	Name string
}

// List is a page's list of related records: the category's products.
type List struct {
	Relation *project.Relation
	// Target is the related entity; Key, its filter by the record (the
	// CategoryID of ProductFilter).
	Target *project.Entity
	Key    string
	// Page is the target's own pages on the site, if it has them.
	Page *Page
}

// Pages are the entities with pages on a site (instance), in the model's
// order.
func Pages(entities []*project.Entity, instance string) []*Page {
	var out []*Page
	byEntity := map[*project.Entity]*Page{}
	for _, e := range entities {
		a := e.Ann[instance]
		if on, _ := a["page"].(bool); !on {
			continue
		}
		pg := &Page{Entity: e, Path: e.Plural.Kebab}
		if s, _ := a["path"].(string); s != "" {
			pg.Path = strings.Trim(s, "/")
		}
		pg.Title = fieldNamed(e, a["title"])
		if pg.Title == nil {
			pg.Title = firstText(e)
		}
		pg.Description = fieldNamed(e, a["description"])
		pg.Image = fieldNamed(e, a["image"])
		if pg.Image == nil {
			for _, f := range e.Fields {
				if f.Type == project.TypeImage {
					pg.Image = f
					break
				}
			}
		}
		pg.Schema, _ = a["schema"].(string)
		pg.Sort, _ = a["sort"].(string)
		pg.Menu, _ = a["menu"].(bool)
		for _, s := range store.ListSorts(e, nil) {
			pg.Sorts = append(pg.Sorts, s.Key, "-"+s.Key)
		}
		pg.Search = len(store.SearchFields(e)) > 0
		for _, f := range store.ListFilters(e) {
			pg.Filters = append(pg.Filters, filtersOf(e, f)...)
		}
		for _, f := range e.Fields {
			if hidden, _ := f.Ann[instance]["hidden"].(bool); hidden || f.Implicit || f.IsFile() || f.Type == project.TypeJSON || f.Type == project.TypeUUID {
				continue
			}
			pg.Fields = append(pg.Fields, f)
		}
		for _, r := range e.Relations {
			if r.OwnsForeignKey() {
				pg.Keys = append(pg.Keys, r)
			}
		}
		byEntity[e] = pg
		out = append(out, pg)
	}
	for _, pg := range out {
		for _, name := range ui.Strings(pg.Entity.Ann[instance]["relations"]) {
			r := relationNamed(pg.Entity, name)
			if r == nil || r.Kind != project.OneToMany || r.Inverse == nil {
				continue
			}
			pg.Lists = append(pg.Lists, &List{Relation: r, Target: r.Target, Key: r.Inverse.Pascal + "ID", Page: byEntity[r.Target]})
		}
	}
	return out
}

// PageOf is an entity's pages on a site, nil when it has none.
func PageOf(entities []*project.Entity, instance string, e *project.Entity) *Page {
	for _, pg := range Pages(entities, instance) {
		if pg.Entity == e {
			return pg
		}
	}
	return nil
}

// Modules are the modules whose entities a site reads, in the model's
// order.
func Modules(entities []*project.Entity, modules []*project.Module, instance string) []*project.Module {
	var out []*project.Module
	add := func(m *project.Module) {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	for _, r := range Redirects(entities, instance) {
		add(r.Entity.Module)
	}
	for _, m := range PageModules(entities, modules, instance) {
		add(m)
	}
	slices.SortStableFunc(out, func(a, b *project.Module) int {
		return slices.Index(modules, a) - slices.Index(modules, b)
	})
	return out
}

// PageModules are the modules whose entities a site's pages read (those it
// redirects by aside), in the model's order.
func PageModules(entities []*project.Entity, modules []*project.Module, instance string) []*project.Module {
	var out []*project.Module
	add := func(m *project.Module) {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	for _, pg := range Pages(entities, instance) {
		add(pg.Entity.Module)
		for _, l := range pg.Lists {
			add(l.Target.Module)
		}
		for _, r := range pg.Keys {
			add(r.Target.Module)
		}
	}
	slices.SortStableFunc(out, func(a, b *project.Module) int {
		return slices.Index(modules, a) - slices.Index(modules, b)
	})
	return out
}

func filtersOf(e *project.Entity, f store.ListFilter) []Filter {
	switch {
	case f.Kind == "in":
		for _, fld := range e.Fields {
			if fld.Pascal == f.Field && fld.Enum != nil {
				return []Filter{{Param: f.Name, Kind: "in", Field: f.Field, Enum: fld.Enum, Ann: fld.Ann, Name: ui.Humanize(fld.Snake)}}
			}
		}
	case f.Kind == "eq" && f.Go == "bool":
		for _, fld := range e.Fields {
			if fld.Pascal == f.Field {
				return []Filter{{Param: f.Name, Kind: "bool", Field: f.Field, Ann: fld.Ann, Name: ui.Humanize(fld.Snake)}}
			}
		}
	case f.Kind == "eq" && f.Go == "uuid.UUID":
		for _, r := range e.Relations {
			if r.OwnsForeignKey() && r.Pascal+"ID" == f.Field {
				return []Filter{{Param: f.Name, Kind: "id", Field: f.Field, Relation: r, Ann: r.Ann, Name: ui.Humanize(r.Snake)}}
			}
		}
	}
	return nil
}

func fieldNamed(e *project.Entity, name any) *project.Field {
	s, _ := name.(string)
	for _, f := range e.Fields {
		if s != "" && f.Snake == s {
			return f
		}
	}
	return nil
}

func firstText(e *project.Entity) *project.Field {
	for _, f := range e.Fields {
		if !f.Implicit && (f.Type == project.TypeString || f.Type == project.TypeText) {
			return f
		}
	}
	return nil
}

func relationNamed(e *project.Entity, name string) *project.Relation {
	for _, r := range e.Relations {
		if r.Snake == name {
			return r
		}
	}
	return nil
}

// realmOf is the realm of a site: its auth option's, or the first.
func realmOf(entities []*project.Entity, opts map[string]any) *security.Realm {
	realms := security.RealmsOf(entities)
	auth, _ := opts["auth"].(string)
	for _, r := range realms {
		if r.Name == auth {
			return r
		}
	}
	if len(realms) > 0 {
		return realms[0]
	}
	return nil
}

// Value is the templ expression that shows field f of the record x as
// people read it: an amount, a date, an enum by its label. rich text
// (@rest(html)), stored sanitized, is written as HTML.
func Value(f *project.Field, x string) string {
	v := x + "." + f.Pascal
	switch f.Type {
	case project.TypeString, project.TypeText:
		if on, _ := f.Ann["rest"]["html"].(bool); on {
			return "@templ.Raw(" + v + ")"
		}
		return "{ " + v + " }"
	case project.TypeInt, project.TypeInt64:
		return "{ Number(int64(" + v + ")) }"
	case project.TypeDecimal, project.TypeUUID:
		return "{ " + v + ".String() }"
	case project.TypeMoney:
		return "{ Money(" + v + ") }"
	case project.TypeBool:
		return "{ YesNo(" + v + ") }"
	case project.TypeDate:
		return "{ Date(" + v + ") }"
	case project.TypeTime:
		return "{ DateTime(" + v + ") }"
	case project.TypeEnum:
		return "{ " + f.Enum.Pascal + "Label(" + v + ") }"
	}
	return "{ fmt.Sprint(" + v + ") }"
}

// Shown is the Go condition under which a page shows field f of x: when it
// has a value.
func Shown(f *project.Field, x string) string {
	v := x + "." + f.Pascal
	switch f.Type {
	case project.TypeString, project.TypeText, project.TypeEnum:
		return v + ` != ""`
	case project.TypeMoney:
		return v + `.CurrencyCode() != ""`
	case project.TypeDate, project.TypeTime:
		return "!" + v + ".IsZero()"
	case project.TypeUUID:
		return v + " != uuid.Nil()"
	}
	return "true"
}

// Plain reports whether a field is short text: what a card shows under its
// title.
func Plain(f *project.Field) bool {
	switch f.Type {
	case project.TypeText, project.TypeJSON, project.TypeUUID:
		return false
	}
	return true
}
