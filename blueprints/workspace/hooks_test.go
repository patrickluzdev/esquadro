package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

// The builds stay out of git, their README.md in: a clone builds, its
// go:embed finding the folder.
func TestIgnoreKeepsTheBuildsReadme(t *testing.T) {
	for name, before := range map[string]string{
		"new":      "",
		"old rule": "/bin/\n\n# The frontends' packages and builds.\nnode_modules/\nweb/*/dist/\n",
		"current":  "node_modules/\nweb/*/dist/*\n!web/*/dist/README.md\n",
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(before), 0o644)
		if _, err := Ignore(context.Background(), hook.Input{ProjectDir: dir}); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if strings.Contains(string(got), "web/*/dist/\n") || strings.Count(string(got), "!web/*/dist/README.md") != 1 || strings.Count(string(got), "node_modules/") != 1 {
			t.Errorf("%s:\n%s", name, got)
		}
		if _, err := exec.LookPath("git"); err != nil {
			continue
		}
		exec.Command("git", "init", "-q", dir).Run()
		for path, ignored := range map[string]bool{"web/admin/dist/README.md": false, "web/site/dist/README.md": false, "web/admin/dist/app/index.html": true, "web/site/dist/assets/site.css": true} {
			err := exec.Command("git", "-C", dir, "check-ignore", "-q", path).Run()
			if (err == nil) != ignored {
				t.Errorf("%s: %s ignored %v, want %v", name, path, err == nil, ignored)
			}
		}
	}
}
