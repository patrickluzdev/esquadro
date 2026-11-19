// Package workspace holds the Go side of the workspace blueprint.
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// distIgnores keep the frontends' packages and builds out of git, all but
// each build folder's README.md, which keeps the folder there.
var distIgnores = []string{"node_modules/", "web/*/dist/*", "!web/*/dist/README.md"}

// Ignore keeps the frontends' packages and builds out of git.
func Ignore(_ context.Context, in hook.Input) (hook.Output, error) {
	var out hook.Output
	if in.DryRun {
		return out, nil
	}
	path := filepath.Join(in.ProjectDir, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	// web/*/dist/ ignored the folder whole, its README.md too, which keeps
	// it in git for go:embed: a clone had no dist, and did not build.
	text := strings.Replace(string(content), "\nweb/*/dist/\n", "\nweb/*/dist/*\n!web/*/dist/README.md\n", 1)
	changed := text != string(content)
	have := map[string]bool{}
	for line := range strings.SplitSeq(text, "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, p := range distIgnores {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		if changed {
			return out, os.WriteFile(path, []byte(text), 0o644)
		}
		return out, nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "# The frontends' packages and builds.\n" + strings.Join(missing, "\n") + "\n"
	return out, os.WriteFile(path, []byte(text), 0o644)
}
