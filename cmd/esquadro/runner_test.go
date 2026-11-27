package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The CI runs on the runner the github_actions blueprint names, and keeps
// what it installs in the runner's temporary folder, so that a machine of
// one's own is left as it was.
func TestCIRunsOnARunnerOfOnesOwn(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a project with the store blueprint, which needs sqlc or Docker")
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	workflow := func() string {
		t.Helper()
		files, _ := filepath.Glob(filepath.Join(dir, ".github", "workflows", "*.yml"))
		if len(files) != 1 {
			t.Fatalf("workflows: %v", files)
		}
		return read(t, files[0])
	}
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	if w := workflow(); strings.Count(w, "runs-on: ubuntu-latest\n") != 2 || !strings.Contains(w, `GOBIN="$RUNNER_TEMP/bin"`) {
		t.Errorf("GitHub's runner by default, esquadro in the temporary folder:\n%s", w)
	}

	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	src := strings.Replace(read(t, appEsq), "  blueprint github_actions\n", "  blueprint github_actions {\n    runner [\"self-hosted\", \"macOS\"]\n  }\n", 1)
	if err := os.WriteFile(appEsq, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	if w := workflow(); strings.Count(w, "runs-on: [self-hosted, macOS]\n") != 2 {
		t.Errorf("the runner of one's own:\n%s", w)
	}
	// A tag's tests are not run again: the image job checks its commit
	// passed on a branch instead.
	w := workflow()
	if !strings.Contains(w, "&& !startsWith(github.ref, 'refs/tags/')") || !strings.Contains(w, "name: The commit passed the CI on a branch") || strings.Contains(w, "needs: test") {
		t.Errorf("a tag runs its tests again:\n%s", w)
	}
	if !strings.Contains(w, "cache: false") || !strings.Contains(w, `GOMODCACHE=$RUNNER_TOOL_CACHE/go-mod`) {
		t.Errorf("a machine of one's own downloads GitHub's cache of Go modules:\n%s", w)
	}
}
