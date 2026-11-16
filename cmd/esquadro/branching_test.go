package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// branchingProject generates a project with the branching blueprint and
// model, in a repository git would start on master: it must start on main.
func branchingProject(t *testing.T, model string) (dir string, run func(args ...string) (string, error)) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the tasks are a POSIX shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	os.WriteFile(gitconfig, []byte("[init]\n\tdefaultBranch = master\n[user]\n\tname = t\n\temail = t@example.com\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	dir = filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-no-gen", "-module", "example.com/shop", "-blueprints", "app,github_actions", dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	src := read(t, appEsq)
	os.WriteFile(appEsq, []byte(strings.Replace(src, "  blueprint git {", "  blueprint branching {\n    model \""+model+"\"\n  }\n  blueprint git {", 1)), 0o644)
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	run = func(args ...string) (string, error) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	return dir, run
}

func mustRun(t *testing.T, run func(args ...string) (string, error), args ...string) string {
	t.Helper()
	out, err := run(args...)
	if err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func branches(t *testing.T, run func(args ...string) (string, error)) string {
	t.Helper()
	return strings.Join(strings.Fields(strings.ReplaceAll(mustRun(t, run, "git", "branch", "--format=%(refname:short)"), "\n", " ")), " ")
}

func TestBranchingGitflow(t *testing.T) {
	dir, run := branchingProject(t, "gitflow")
	if got := branches(t, run); got != "develop main" {
		t.Fatalf("branches: %q", got)
	}
	if got := strings.TrimSpace(mustRun(t, run, "git", "config", "--get", "gitflow.branch.develop")); got != "develop" {
		t.Fatalf("git-flow config: %q", got)
	}
	ci := read(t, filepath.Join(dir, ".github", "workflows", "ci.yml"))
	if !strings.Contains(ci, `branches: ["main", "develop"]`) || !strings.Contains(ci, "does not follow gitflow") {
		t.Fatalf("ci.yml:\n%s", ci)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "CONTRIBUTING.md")), "## Branches: GitFlow") {
		t.Fatal("CONTRIBUTING.md does not explain gitflow")
	}

	flow := func(args ...string) string {
		return mustRun(t, run, append([]string{"sh", "scripts/branching.sh"}, args...)...)
	}
	flow("feature-start", "login")
	os.WriteFile(filepath.Join(dir, "login.txt"), []byte("x\n"), 0o644)
	mustRun(t, run, "git", "add", ".")
	mustRun(t, run, "git", "commit", "-qm", "feat: login")
	flow("feature-finish")
	flow("release-start", "1.0.0")
	if out := flow("release-finish"); !strings.Contains(out, "git push origin main develop v1.0.0") {
		t.Fatalf("release-finish: %s", out)
	}
	if got := mustRun(t, run, "git", "tag", "--points-at", "main"); strings.TrimSpace(got) != "v1.0.0" {
		t.Fatalf("tag on main: %q", got)
	}
	if _, err := run("git", "merge-base", "--is-ancestor", "feature/login", "main"); err == nil {
		t.Fatal("feature/login was not deleted")
	}
	mustRun(t, run, "git", "show", "main:login.txt")
	if got := branches(t, run); got != "develop main" {
		t.Fatalf("branches after the release: %q", got)
	}

	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{"release-start", "1.0"}, `"1.0" is not a version`},
		{[]string{"feature-finish"}, "not on a feature/ branch"},
		{[]string{"feature-start"}, "give it a name"},
		{[]string{"nope"}, `unknown command "nope"`},
	} {
		out, err := run(append([]string{"sh", "scripts/branching.sh"}, tc.args...)...)
		if err == nil || !strings.Contains(out, tc.msg) {
			t.Errorf("%v: %v %q", tc.args, err, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x\n"), 0o644)
	if out, err := run("sh", "scripts/branching.sh", "feature-start", "x"); err == nil || !strings.Contains(out, "commit or stash") {
		t.Errorf("dirty tree: %v %q", err, out)
	}
}

func TestBranchingOtherModels(t *testing.T) {
	t.Run("github_flow", func(t *testing.T) {
		dir, run := branchingProject(t, "github_flow")
		if got := branches(t, run); got != "main" {
			t.Fatalf("branches: %q", got)
		}
		if !strings.Contains(read(t, filepath.Join(dir, ".github", "workflows", "ci.yml")), `branches: ["main"]`) {
			t.Fatal("ci.yml branches")
		}
		mustRun(t, run, "sh", "scripts/branching.sh", "feature-start", "login")
		if got := strings.TrimSpace(mustRun(t, run, "git", "branch", "--show-current")); got != "feature/login" {
			t.Fatalf("current branch: %q", got)
		}
	})
	t.Run("gitlab_flow", func(t *testing.T) {
		_, run := branchingProject(t, "gitlab_flow")
		if got := branches(t, run); got != "main production staging" {
			t.Fatalf("branches: %q", got)
		}
		mustRun(t, run, "git", "commit", "-q", "--allow-empty", "-m", "feat: x")
		mustRun(t, run, "sh", "scripts/branching.sh", "promote", "staging")
		mustRun(t, run, "git", "merge-base", "--is-ancestor", "main", "staging")
		if out, err := run("sh", "scripts/branching.sh", "promote", "qa"); err == nil || !strings.Contains(out, "qa is not an environment") {
			t.Fatalf("promote qa: %v %q", err, out)
		}
	})
	t.Run("trunk", func(t *testing.T) {
		_, run := branchingProject(t, "trunk")
		if out := mustRun(t, run, "sh", "scripts/branching.sh", "sync"); !strings.Contains(out, "main is up to date") {
			t.Fatalf("sync: %q", out)
		}
	})
	t.Run("release_flow", func(t *testing.T) {
		dir, run := branchingProject(t, "release_flow")
		if !strings.Contains(read(t, filepath.Join(dir, ".github", "workflows", "ci.yml")), `branches: ["main", "releases/**"]`) {
			t.Fatal("ci.yml branches")
		}
		mustRun(t, run, "sh", "scripts/branching.sh", "release-cut", "1.2")
		os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("y\n"), 0o644)
		mustRun(t, run, "git", "add", ".")
		mustRun(t, run, "git", "commit", "-q", "-m", "fix: y")
		fix := strings.TrimSpace(mustRun(t, run, "git", "rev-parse", "HEAD"))
		mustRun(t, run, "sh", "scripts/branching.sh", "release-pick", fix, "1.2")
		if log := mustRun(t, run, "git", "log", "-1", "--format=%B", "releases/1.2"); !strings.Contains(log, "cherry picked from commit "+fix) {
			t.Fatalf("releases/1.2: %q", log)
		}
	})
}

func TestBranchingRefusesAnUnknownModel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := filepath.Join(t.TempDir(), "shop")
	esq(t, "new", "-no-gen", "-no-git", "-module", "example.com/shop", dir)
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	os.WriteFile(appEsq, []byte(strings.Replace(read(t, appEsq), "  blueprint app\n", "  blueprint app\n  blueprint git\n  blueprint branching {\n    model \"oneflow\"\n  }\n", 1)), 0o644)
	if code, out, errOut := esq(t, "gen", dir); code == 0 || !strings.Contains(out+errOut, `model "oneflow" is not one of gitflow`) {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
}
