package compose

import (
	"path"
	"strings"
	"text/template"
)

// The project's place in its repository: the git blueprint's path option
// names the folder the project is in, relative to the repository's root
// ("" when the project is the repository). Files of the repository, such as
// its CI workflows, are written at the root, outside the project.

// RepositoryPath returns the folder of the project in its repository,
// cleaned, from the options of the git blueprint; "" when the project is at
// the root, or when the app does not use git.
func RepositoryPath(gitOptions map[string]any) string {
	p, _ := gitOptions["path"].(string)
	p = strings.Trim(path.Clean("/"+p), "/")
	return p
}

// repositoryFuncs are the template helpers about the project's place in its
// repository.
func repositoryFuncs(repoPath string) template.FuncMap {
	root := repoRootOf(repoPath)
	return template.FuncMap{
		// repoRoot is the way from the project to the root of its
		// repository, e.g. "../", or "": the start of a template's file
		// name for a file of the repository.
		"repoRoot": func() string { return root },
		// repoPath is the project's folder in its repository, e.g. "api",
		// or "".
		"repoPath": func() string { return repoPath },
		// inRepo is a path of the project as seen from the repository's
		// root, as CI steps see it: inRepo "clients/ts" is
		// "api/clients/ts".
		"inRepo": func(p string) string {
			if repoPath == "" {
				return p
			}
			if p == "" || p == "." {
				return repoPath
			}
			return repoPath + "/" + p
		},
		// tagPrefix starts the project's release tags, e.g. "api/" for
		// api/v1.2.3, so that each project of a repository has its own;
		// "" at the root.
		"tagPrefix": func() string {
			if repoPath == "" {
				return ""
			}
			return repoPath + "/"
		},
		// workflowName names the project's CI workflow: "ci" at the root,
		// else after its folder ("api", "apps-api").
		"workflowName": func() string {
			if repoPath == "" {
				return "ci"
			}
			return strings.ReplaceAll(repoPath, "/", "-")
		},
	}
}

// cleanRepoTarget validates a generated file path that may be a file of the
// repository: one starting with root (the way up to the repository's root)
// must stay inside the repository and out of the project. It returns the
// path to record, relative to the project.
func cleanRepoTarget(p, root, repoPath string) (string, bool) {
	if root == "" || !strings.HasPrefix(p, root) {
		return CleanTarget(p)
	}
	rest, ok := CleanTarget(strings.TrimPrefix(p, root))
	if !ok || rest == repoPath || strings.HasPrefix(rest, repoPath+"/") {
		return "", false
	}
	return root + rest, true
}

// repoRootOf is the way up from a project in repoPath to its repository's
// root: "../" for "api", "../../" for "apps/api", "" for the root.
func repoRootOf(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	return strings.Repeat("../", strings.Count(repoPath, "/")+1)
}

// repoPath is the project's folder in its repository.
func (c *composer) repoPath() string { return RepositoryPath(c.set.Options["git"]) }
