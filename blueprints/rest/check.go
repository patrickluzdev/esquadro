package rest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Paginations are the ways a list pages.
var Paginations = []string{"cursor", "page"}

// Check checks the paginations the app and its entities name.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	bad := func(list []string) string {
		for _, v := range list {
			if !slices.Contains(Paginations, v) {
				return v
			}
		}
		return ""
	}
	if v := bad(strs(set.Options["rest"]["pagination"])); v != "" {
		for _, r := range p.App.Blueprints {
			if r.Name == "rest" {
				diags.Errorf(r.Pos, r.Pos, "pagination %q is not cursor or page", v)
			}
		}
	}
	for _, e := range p.Entities {
		if v := bad(strs(e.Ann["rest"]["pagination"])); v != "" {
			diags.Errorf(e.Pos, e.Pos, "%s: pagination %q is not cursor or page", e.Raw, v)
		}
		if n, ok := e.Ann["rest"]["unpaged_max"].(int64); ok && n < 1 {
			diags.Errorf(e.Pos, e.Pos, "%s: unpaged_max %d must be at least 1", e.Raw, n)
		}
	}
	for _, e := range p.Entities {
		diags = append(diags, collisions(e)...)
	}
	modes := []string{"optional", "required"}
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			if f.Pattern != "" && !PortablePattern(f.Pattern) {
				diags.Warnf(f.Pos, f.Pos, "%s.%s: the contract leaves out its pattern, which JavaScript reads differently ((?flags), \\A, \\z, \\p{...}); the API still checks it", e.Raw, f.Raw)
			}
		}
	}
	for _, key := range []string{"if_match", "idempotency"} {
		if v, ok := set.Options["rest"][key].(string); ok && !slices.Contains(modes, v) {
			diags.Errorf(p.App.Pos, p.App.Pos, "%s %q is not optional or required", key, v)
		}
		for _, e := range p.Entities {
			if v, ok := e.Ann["rest"][key].(string); ok && !slices.Contains(modes, v) {
				diags.Errorf(e.Pos, e.Pos, "%s: %s %q is not optional or required", e.Raw, key, v)
			}
		}
	}
	if n, ok := set.Options["rest"]["unpaged_max"].(int64); ok && n < 1 {
		diags.Errorf(p.App.Pos, p.App.Pos, "%s", fmt.Sprintf("unpaged_max %d must be at least 1", n))
	}
	return diags
}

// strs is a list option or annotation value as strings.
func strs(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return nil
}

// IfMatchRequired reports whether changing e needs If-Match.
func IfMatchRequired(e *project.Entity, opts map[string]any) bool {
	if v, ok := e.Ann["rest"]["if_match"].(string); ok {
		return v == "required"
	}
	v, _ := opts["if_match"].(string)
	return v == "required"
}

// IdempotencyRequired reports whether creating e needs an Idempotency-Key.
func IdempotencyRequired(e *project.Entity, opts map[string]any) bool {
	if v, ok := e.Ann["rest"]["idempotency"].(string); ok {
		return v == "required"
	}
	v, _ := opts["idempotency"].(string)
	return v == "required"
}

// ListParams are the query parameters every list has, besides its filters.
var ListParams = []string{"page", "per_page", "limit", "after", "sort", "q", "fields", "include"}

// collisions are the filters of e's list whose parameter is one of the
// list's own (a field page with @store(filter)), or another filter's (a
// field price_min beside price filtered by range): the contract cannot
// hold both, and the API's code would not generate.
func collisions(e *project.Entity) diag.List {
	var diags diag.List
	seen := map[string]bool{}
	for _, f := range APIFilters(e) {
		pos := paramPos(e, f.Param)
		switch {
		case slices.Contains(ListParams, f.Param):
			diags.Errorf(pos, pos, "%s: a filter is the list's parameter %s (%s): rename the field, or leave it out of the filters", e.Raw, f.Param, strings.Join(ListParams, ", "))
		case seen[f.Param]:
			diags.Errorf(pos, pos, "%s: two filters are the parameter %s (a range's _min and _max, and a field of that name): rename the field, or leave one out of the filters", e.Raw, f.Param)
		}
		seen[f.Param] = true
	}
	return diags
}

// paramPos is where the field a filter parameter comes from is declared:
// price for price_min, category for category_id.
func paramPos(e *project.Entity, param string) diag.Pos {
	for _, f := range e.Fields {
		if f.Snake == param {
			return f.Pos
		}
	}
	for _, r := range e.Relations {
		if r.Snake+"_id" == param && r.OwnsForeignKey() {
			return r.Pos
		}
	}
	base := strings.TrimSuffix(strings.TrimSuffix(param, "_min"), "_max")
	for _, f := range e.Fields {
		if f.Snake == base && f.Pos.IsValid() {
			return f.Pos
		}
	}
	return e.Pos
}
