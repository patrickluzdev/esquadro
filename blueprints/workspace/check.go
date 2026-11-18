package workspace

import (
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check refuses a layout esquadro does not generate yet.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	layout, _ := set.Options["workspace"]["layout"].(string)
	if layout == "monorepo" {
		return nil
	}
	for _, r := range p.App.Blueprints {
		if r.Name == "workspace" {
			diags.Errorf(r.Pos, r.Pos, "layout %s is not one esquadro generates yet: monorepo is", layout).
				Suggestion = "layout monorepo"
		}
	}
	return diags
}
