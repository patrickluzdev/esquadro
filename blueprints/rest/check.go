package rest

import (
	"fmt"
	"slices"

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
	modes := []string{"optional", "required"}
	if v, ok := set.Options["rest"]["if_match"].(string); ok && !slices.Contains(modes, v) {
		diags.Errorf(p.App.Pos, p.App.Pos, "if_match %q is not optional or required", v)
	}
	for _, e := range p.Entities {
		if v, ok := e.Ann["rest"]["if_match"].(string); ok && !slices.Contains(modes, v) {
			diags.Errorf(e.Pos, e.Pos, "%s: if_match %q is not optional or required", e.Raw, v)
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
