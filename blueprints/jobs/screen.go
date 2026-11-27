package jobs

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// reservedPaths are the API's own paths, which the screen may not take.
var reservedPaths = []string{"/api", "/healthz", "/readyz", "/docs", "/storage", "/.well-known"}

// checkScreen checks the ui and ui_access options: a path of its own, and
// sign-in to guard it, the security blueprint with a principal, whose
// realms and roles the rules name.
func checkScreen(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	if set == nil {
		return diags
	}
	for _, b := range set.Blueprints {
		if b.Manifest.Name != "jobs" {
			continue
		}
		opts := set.Options["jobs"]
		at := func(key string) diag.Pos {
			for _, o := range b.Ref.Options {
				if o.Key.Name == key {
					return o.Pos
				}
			}
			return b.Ref.Pos
		}
		path, _ := opts["ui"].(string)
		rules := ui.Strings(opts["ui_access"])
		if path == "" {
			if len(rules) > 0 {
				diags.Errorf(at("ui_access"), at("ui_access"), "ui_access says who may open the jobs screen, but there is none: set ui too").
					Suggestion = `ui "/jobs"`
			}
			continue
		}
		if !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.ContainsAny(path, " {}?#*") {
			diags.Errorf(at("ui"), at("ui"), "ui: %q is not a path such as /jobs", path).Suggestion = `ui "/jobs"`
		}
		for _, reserved := range reservedPaths {
			if path == reserved || strings.HasPrefix(path, reserved+"/") {
				diags.Errorf(at("ui"), at("ui"), "ui: %s is the API's own; take another path, such as /jobs", path)
			}
		}
		if prefix, _ := set.Options["security"]["path"].(string); prefix != "" && (path == prefix || strings.HasPrefix(path, prefix+"/")) {
			diags.Errorf(at("ui"), at("ui"), "ui: %s is under the identity routes (%s); take another path, such as /jobs", path, prefix)
		}
		realms := security.RealmsOf(p.Entities)
		if !slices.ContainsFunc(set.Blueprints, func(x *blueprint.Blueprint) bool { return x.Manifest.Name == "security" }) || len(realms) == 0 {
			diags.Errorf(at("ui"), at("ui"), "the jobs screen needs sign-in to guard it: add the security blueprint and an entity with @security(principal), or take ui away")
			continue
		}
		if !slices.ContainsFunc(set.Blueprints, func(x *blueprint.Blueprint) bool { return x.Manifest.Name == "http" }) {
			diags.Errorf(at("ui"), at("ui"), "the jobs screen is served over HTTP: add the http blueprint, or take ui away")
		}
		for _, rule := range rules {
			name, role, hasRole := strings.Cut(rule, ".")
			i := slices.IndexFunc(realms, func(r *security.Realm) bool { return r.Name == name })
			switch {
			case i < 0:
				diags.Errorf(at("ui_access"), at("ui_access"), "ui_access: %s is no realm of the app (an entity with @security(principal))", rule)
			case hasRole && !slices.Contains(realms[i].Roles, role):
				diags.Errorf(at("ui_access"), at("ui_access"), "ui_access: %s has no role %s", name, role)
			}
		}
	}
	return diags
}
