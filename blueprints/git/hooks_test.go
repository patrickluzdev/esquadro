package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/hook"
)

func commits(t *testing.T, dir string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		return 0
	}
	n := strings.TrimSpace(string(out))
	return map[string]int{"1": 1, "2": 2}[n]
}

func project(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInitCommitsTheFirstGenerationOnly(t *testing.T) {
	dir := project(t)
	in := hook.Input{ProjectDir: dir, Options: map[string]any{"initial_commit": true, "message": "chore: first"}}
	if _, err := Init(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if commits(t, dir) != 1 {
		t.Fatal("no first commit")
	}
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--format=%s").Output()
	if strings.TrimSpace(string(out)) != "chore: first" {
		t.Fatalf("message %q", out)
	}
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	if _, err := Init(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if commits(t, dir) != 1 {
		t.Fatal("a later generation committed")
	}
}

func TestInitWithoutInitialCommit(t *testing.T) {
	dir := project(t)
	in := hook.Input{ProjectDir: dir, Options: map[string]any{"initial_commit": false}}
	if _, err := Init(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatal("no repository")
	}
	if commits(t, dir) != 0 {
		t.Fatal("committed although initial_commit is false")
	}
}
