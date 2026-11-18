// Package workspace holds the Go side of the workspace blueprint.
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

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
	have := map[string]bool{}
	for line := range strings.SplitSeq(string(content), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, p := range []string{"node_modules/", "web/*/dist/"} {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	text := string(content)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "# The frontends' packages and builds.\n" + strings.Join(missing, "\n") + "\n"
	return out, os.WriteFile(path, []byte(text), 0o644)
}
