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
		"sitePages": Pages,
		"siteBrand": brandOf,
		"hasSuffix": strings.HasSuffix,
		"siteBrandDir": func(opts map[string]any) string {
			if dir, _ := opts["brand"].(string); strings.Trim(dir, "/") != "" {
				return strings.Trim(dir, "/")
			}
			return "brand"
		},
		"siteRedirects": Redirects,
		"siteRedirect":  RedirectOf,
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
		"siteHomeModules":        HomeModules,
		"siteLabel":              func(a project.Ann, instance, fallback string) string { return ui.Label(a, instance, fallback) },
		"humanize":               ui.Humanize,
		"orders":                 store.Orders,
		"written":                store.Written,
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
		"siteOG": func(pages []*Page) bool {
			return slices.ContainsFunc(pages, func(p *Page) bool { return p.OG })
		},
		"siteOGPictures": func(pages []*Page) bool {
			return slices.ContainsFunc(pages, func(p *Page) bool { return p.OG && p.Image != nil })
		},
		"ogColors":        OGColors,
		"ogThemeHash":     OGThemeHash,
		"ogFont":          OGFont,
		"ogLogo":          OGLogo,
		"ogPublic":        ogPublic,
		"ogSample":        ogSample,
		"ogParents":       ogParents,
		"ogSampleImports": ogSampleImports,
		"ogTested":        ogTested,
		"ogPackages":      ogPackages,
		"siteKeys": func(pages []*Page) bool {
			return slices.ContainsFunc(pages, func(p *Page) bool { return p.Key != nil })
		},
		"siteBrowseOf": func(pg *Page, of any) *Browse {
			for _, b := range pg.Browses {
				if f, ok := of.(*project.Field); ok && b.Kind == "in" && b.Field == f.Pascal {
					return b
				}
				if r, ok := of.(*project.Relation); ok && b.Relation == r {
					return b
				}
			}
			return nil
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
			add := func(e *project.Enum) {
				if e != nil && !slices.Contains(out, e) {
					out = append(out, e)
				}
			}
			for _, p := range pages {
				for _, f := range p.Fields {
					add(f.Enum)
				}
				for _, b := range p.Browses {
					add(b.Enum)
				}
				if p.Seller != nil {
					add(p.Seller.Enum)
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
	// OG gives each record an Open Graph image drawn by the app
	// (/<path>/<id>/og.png), with the images blueprint.
	OG bool
	// Fields are those the site shows, in order: none hidden, no files.
	Fields []*project.Field
	// Keys are its relations to one record the site shows.
	Keys []*project.Relation
	// Key is the unique field that ends a record's address
	// (/<path>/<slug>-<key>), nil for the address by id.
	Key *project.Field
	// Browses are its list's pages of one filter's value: an enum's, a
	// related record's.
	Browses []*Browse
	// Available is the enum field a record is in stock by, with the values
	// it is in stock with (schema product); nil for none.
	Available       *project.Field
	AvailableValues []*project.EnumValue
	// Seller is the field of who sells a record (schema product), nil for
	// none.
	Seller *project.Field
	// Price is the money field of a product's offer, nil for none.
	Price *project.Field
}

// Browse is a list's page of one value of a filter (@site(browse)): of a
// related record, /<path>/<slug>, or of an enum's value,
// /<path>/<segment>/<slug>.
type Browse struct {
	Filter
	// Pascal names it in Go: Category, Status.
	Pascal string
	// Segment is the part of the address before the value's slug, "" for
	// none (a relation's only).
	Segment string
	// Title is the field the related record is called by (its slug).
	Title *project.Field
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
		pg.OG, _ = a["og_image"].(bool)
		for _, s := range store.ListSorts(e, nil) {
			pg.Sorts = append(pg.Sorts, s.Key, "-"+s.Key)
		}
		// An order worked out by an expression, the largest first.
		for _, o := range store.Orders(e) {
			pg.Sorts = append(pg.Sorts, o.Snake)
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
		pg.Key = fieldNamed(e, a["key"])
		pg.Seller = fieldNamed(e, a["seller"])
		for _, f := range pg.Fields {
			if pg.Price == nil && f.Type == project.TypeMoney {
				pg.Price = f
				break
			}
		}
		for _, f := range e.Fields {
			if values := ui.Strings(f.Ann[instance]["available"]); len(values) > 0 && f.Enum != nil && pg.Available == nil {
				pg.Available = f
				for _, v := range values {
					if ev := f.Enum.Value(v); ev != nil {
						pg.AvailableValues = append(pg.AvailableValues, ev)
					}
				}
			}
		}
		pg.Browses = browsesOf(pg, instance)
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
// redirects by aside), in the model's order: of the pages of the entities
// given, or of all of them.
func PageModules(entities []*project.Entity, modules []*project.Module, instance string, of ...*project.Entity) []*project.Module {
	var out []*project.Module
	add := func(m *project.Module) {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	for _, pg := range Pages(entities, instance) {
		if len(of) > 0 && !slices.Contains(of, pg.Entity) {
			continue
		}
		add(pg.Entity.Module)
		for _, l := range pg.Lists {
			add(l.Target.Module)
		}
		for _, r := range pg.Keys {
			add(r.Target.Module)
		}
		for _, f := range pg.Filters {
			if f.Enum != nil {
				add(f.Enum.Module)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b *project.Module) int {
		return slices.Index(modules, a) - slices.Index(modules, b)
	})
	return out
}

// HomeModules are the modules of the entities with pages on a site, which
// its first page, search and sitemap read, in the model's order.
func HomeModules(entities []*project.Entity, modules []*project.Module, instance string) []*project.Module {
	var out []*project.Module
	for _, m := range modules {
		for _, pg := range Pages(entities, instance) {
			if pg.Entity.Module == m {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// browsesOf are the browse pages of a page's filters, in its filters'
// order: those of an enum or a related record with a title marked
// @<instance>(browse).
func browsesOf(pg *Page, instance string) []*Browse {
	var out []*Browse
	for _, f := range pg.Filters {
		if on, _ := f.Ann[instance]["browse"].(bool); !on {
			continue
		}
		b := &Browse{Filter: f}
		b.Segment, _ = f.Ann[instance]["path"].(string)
		b.Segment = strings.Trim(b.Segment, "/")
		switch f.Kind {
		case "in":
			for _, fld := range pg.Entity.Fields {
				if fld.Pascal == f.Field {
					b.Pascal = fld.Pascal
					if b.Segment == "" {
						b.Segment = fld.Kebab
					}
				}
			}
		case "id":
			b.Pascal = f.Relation.Pascal
			b.Title = TitleOf(f.Relation.Target, instance)
			if b.Title == nil {
				continue
			}
		default:
			continue
		}
		out = append(out, b)
	}
	return out
}

// TitleOf is the field a record of e is called by on a site: its pages'
// title, or its first text field; nil for none.
func TitleOf(e *project.Entity, instance string) *project.Field {
	if f := fieldNamed(e, e.Ann[instance]["title"]); f != nil {
		return f
	}
	return firstText(e)
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

// brandOf is a site's brand, read from the project of app; nil without one
// (or when it cannot be read: the check says why).
func brandOf(app *project.App, opts map[string]any) *ui.Brand {
	if app == nil {
		return nil
	}
	b, err := ui.BrandOf(app.Pos.File, opts)
	if err != nil {
		return nil
	}
	return b
}

// CrumbBrowse is the browse page a record's breadcrumb goes through, after
// its list: of its first related record with one, or else of its first
// enum with one; nil for none.
func (p *Page) CrumbBrowse() *Browse {
	for _, b := range p.Browses {
		if b.Relation != nil {
			return b
		}
	}
	if len(p.Browses) > 0 {
		return p.Browses[0]
	}
	return nil
}
