package clientts

import (
	"slices"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check refuses a client without a package name, or with an unknown
// registry.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	opts := set.Options["client_ts"]
	if PackageName(p.App, opts) == "" {
		diags.Errorf(p.App.Pos, p.App.Pos, "client_ts: the module is not on GitHub, so the client's package needs a scope: blueprint client_ts { scope \"you\" }")
	}
	if r, _ := opts["registry"].(string); r != "" && !slices.Contains([]string{"github", "npmjs"}, r) {
		diags.Errorf(p.App.Pos, p.App.Pos, "client_ts: registry %q is not github or npmjs", r)
	}
	return diags
}
