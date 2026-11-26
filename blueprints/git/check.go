package git

import (
	"path"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	model "github.com/patrickluzdev/esquadro/internal/project"
)

// Check refuses a path that is not a folder inside the repository, and
// what esquadro does not generate yet for a project in one: a platform, and
// branching, whose tasks and rules are the whole repository's.
func Check(p *model.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	raw, _ := set.Options["git"]["path"].(string)
	if raw == "" {
		return nil
	}
	for _, r := range p.App.Blueprints {
		if r.Name != "git" {
			continue
		}
		clean := path.Clean(raw)
		switch {
		case strings.HasPrefix(raw, "/") || strings.Contains(raw, `\`) || clean != raw ||
			clean == "." || clean == ".." || strings.HasPrefix(clean, "../"):
			diags.Errorf(r.Pos, r.Pos, "path %q is not a folder inside the repository", raw).
				Suggestion = `a relative path without .. or a trailing slash, e.g. path "api"`
		case p.Kind == "platform":
			diags.Errorf(r.Pos, r.Pos, "a platform in a folder of its repository is not one esquadro generates yet").
				Suggestion = "give the platform a repository of its own"
		case set.Options["branching"] != nil:
			diags.Errorf(r.Pos, r.Pos, "branching in a project inside a folder of its repository is not one esquadro generates yet: its tasks and rules are the whole repository's").
				Suggestion = "remove blueprint branching, or the path"
		}
	}
	return diags
}

// repoRootOf is the way up from a project in path to its repository's root.
func repoRootOf(p string) string {
	if p == "" {
		return ""
	}
	return strings.Repeat("../", strings.Count(p, "/")+1)
}
