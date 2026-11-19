package store

import (
	"slices"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// SearchLanguages are the text search configurations PostgreSQL has
// built in.
var SearchLanguages = []string{"simple", "arabic", "armenian", "basque", "catalan", "danish", "dutch", "english", "finnish",
	"french", "german", "greek", "hindi", "hungarian", "indonesian", "irish", "italian", "lithuanian", "nepali",
	"norwegian", "portuguese", "romanian", "russian", "serbian", "spanish", "swedish", "tamil", "turkish", "yiddish"}

// Check checks what @store says of fields: search on text only, sort on
// required fields that can be ordered, and the search language.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	checkSingle(p, &diags)
	checkImports(p, &diags)
	checkTotals(p, &diags)
	checkFlows(p, set, &diags)
	checkHistories(p, &diags)
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			a := f.Ann["store"]
			at := func(key string) dsl.Span {
				for _, an := range f.Annotations {
					if an.Name.Name != "store" {
						continue
					}
					for _, arg := range an.Args {
						if arg.Key.Name == key {
							return arg.Span
						}
					}
					return an.Span
				}
				return dsl.Span{Pos: f.Pos, End: f.Pos}
			}
			if on, _ := a["search"].(bool); on && f.Type != project.TypeString && f.Type != project.TypeText {
				s := at("search")
				diags.Errorf(s.Pos, s.End, "search: %s.%s is %s; only string and text fields are searched", e.Raw, f.Raw, f.Type)
			}
			if on, _ := a["sort"].(bool); on {
				s := at("sort")
				switch {
				case f.Type == project.TypeJSON:
					diags.Errorf(s.Pos, s.End, "sort: %s.%s is json, which has no order", e.Raw, f.Raw)
				case !f.Required && f.Default == nil && !IsTotal(f):
					diags.Errorf(s.Pos, s.End, "sort: %s.%s may be empty, and a page ordered by it would skip the empty ones: make it required or give it a default", e.Raw, f.Raw)
				}
			}
			if on, _ := a["filter"].(bool); on && f.Type == project.TypeJSON {
				s := at("filter")
				diags.Errorf(s.Pos, s.End, "filter: %s.%s is json, which lists do not filter by", e.Raw, f.Raw)
			}
		}
	}
	if lang, _ := set.Options["store"]["search_language"].(string); lang != "" && !slices.Contains(SearchLanguages, lang) {
		for _, r := range p.App.Blueprints {
			if r.Name == "store" {
				diags.Errorf(r.Pos, r.Pos, "search_language %q is not one of PostgreSQL's: %v", lang, SearchLanguages)
			}
		}
	}
	return diags
}
