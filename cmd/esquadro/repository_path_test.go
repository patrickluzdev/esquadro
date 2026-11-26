package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// repoProject generates a project in the folder api of a repository that
// holds more than it, with the given blueprints and the git path option.
func repoProject(t *testing.T, blueprints string) (repo, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git and the CI are POSIX here")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	os.WriteFile(gitconfig, []byte("[init]\n\tdefaultBranch = main\n[user]\n\tname = t\n\temail = t@example.com\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	repo = t.TempDir()
	dir = filepath.Join(repo, "api")
	if code, _, errOut := esq(t, "new", "-no-gen", "-module", "example.com/shop", "-blueprints", blueprints, dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	src := read(t, appEsq)
	if !strings.Contains(src, "  blueprint git {") {
		t.Fatalf("no git block to give a path:\n%s", src)
	}
	os.WriteFile(appEsq, []byte(strings.Replace(src, "  blueprint git {", "  blueprint git {\n    path \"api\"", 1)), 0o644)
	return repo, dir
}

func TestGenInAFolderOfTheRepository(t *testing.T) {
	repo, dir := repoProject(t, "app,docker,deploy,github_actions")
	// The repository already exists, with the other parts of it.
	writeFile(t, repo, "site/index.html", "<p>site</p>\n")
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("a repository was created inside the project")
	}
	if _, err := os.Stat(filepath.Join(dir, ".github")); err == nil {
		t.Error("the workflow was written inside the project")
	}
	ci := read(t, filepath.Join(repo, ".github", "workflows", "api.yml"))
	for _, want := range []string{
		"name: api",
		`tags: ["api/v*"]`,
		`paths: ["api/**", ".github/workflows/api.yml"]`,
		"working-directory: api",
		"go-version-file: api/go.mod",
		"cache-dependency-path: api/go.sum",
		"startsWith(github.ref, 'refs/tags/api/v')",
		`type=match,pattern=api/v(\d+\.\d+\.\d+.*),group=1`,
		"context: api",
		`"${GITHUB_REF_NAME#api/}"`,
	} {
		if !strings.Contains(ci, want) {
			t.Errorf("the workflow lacks %s:\n%s", want, ci)
		}
	}
	if strings.Contains(ci, "type=semver") {
		t.Errorf("semver tags read v1.2.3, not api/v1.2.3:\n%s", ci)
	}

	lock, err := workspace.ReadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lock.Files["../.github/workflows/api.yml"]; !ok {
		t.Errorf("the workflow is not in the lockfile: %v", lock.Files)
	}
	if code, out, errOut := esq(t, "gen", "-check", dir); code != 0 {
		t.Fatalf("gen -check after gen: %s%s", out, errOut)
	}

	// Back to the root: the workflow returns to the project and the one of
	// the repository goes, as any file no longer generated.
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	os.WriteFile(appEsq, []byte(strings.Replace(read(t, appEsq), "    path \"api\"\n", "", 1)), 0o644)
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen at the root: %s%s", out, errOut)
	}
	if _, err := os.Stat(filepath.Join(repo, ".github", "workflows", "api.yml")); err == nil {
		t.Error("the repository's workflow stayed")
	}
	if ci := read(t, filepath.Join(dir, ".github", "workflows", "ci.yml")); !strings.Contains(ci, `tags: ["v*"]`) || strings.Contains(ci, "working-directory") {
		t.Errorf("the workflow at the root is not the usual one:\n%s", ci)
	}
}

func TestGenInAFolderCreatesTheRepositoryAtTheRoot(t *testing.T) {
	repo, dir := repoProject(t, "app")
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Errorf("no repository at the root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("a repository was created inside the project")
	}
}

func TestGenInAFolderRefusesWhatItCannotDo(t *testing.T) {
	for _, tc := range []struct{ name, path, extra, want string }{
		{"outside", "../api", "", "is not a folder inside the repository"},
		{"absolute", "/api", "", "is not a folder inside the repository"},
		{"trailing slash", "api/", "", "is not a folder inside the repository"},
		{"branching", "api", "  blueprint branching\n", "branching in a project inside a folder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, dir := repoProject(t, "app,github_actions")
			appEsq := filepath.Join(dir, "esquadro", "app.esq")
			src := strings.Replace(read(t, appEsq), `path "api"`, `path "`+tc.path+`"`, 1)
			src = strings.Replace(src, "  blueprint git {", tc.extra+"  blueprint git {", 1)
			os.WriteFile(appEsq, []byte(src), 0o644)
			code, out, errOut := esq(t, "gen", dir)
			if code == 0 || !strings.Contains(out+errOut, tc.want) {
				t.Fatalf("gen: code %d, want %q in:\n%s%s", code, tc.want, out, errOut)
			}
		})
	}
}

func TestGenInAFolderWithFrontends(t *testing.T) {
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	os.WriteFile(gitconfig, []byte("[user]\n\tname = t\n\temail = t@example.com\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// The project is in shop/ of its repository: spaProject's folder.
	dir, code, errOut := spaProject(t, func(app string) string {
		return strings.Replace(app, "  blueprint api_docs\n", "  blueprint api_docs\n  blueprint git {\n    path \"shop\"\n    initial_commit false\n  }\n", 1)
	})
	if code != 0 {
		t.Fatal(errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "web", "admin", "package.json")); err != nil {
		t.Errorf("the frontend is not in the project: %v", err)
	}
	ci := read(t, filepath.Join(filepath.Dir(dir), ".github", "workflows", "shop.yml"))
	for _, want := range []string{
		"working-directory: shop/clients/ts",
		"working-directory: shop\n",
		"The frontends' packages",
	} {
		if !strings.Contains(ci, want) {
			t.Errorf("the workflow lacks %q:\n%s", want, ci)
		}
	}
}
