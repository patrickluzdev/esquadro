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

// greetRepo makes a git repository with a blueprint whose template and
// post-gen hook both say version, and returns its file URL and a function
// that commits the blueprint saying another version on main.
func greetRepo(t *testing.T) (string, func(version string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(version string) {
		files := map[string]string{
			"greet/blueprint.esq":               "blueprint greet {\n  version \"" + version + "\"\n  hook post-gen \"hooks/stamp\" writes \"stamp.txt\"\n}\n",
			"greet/templates/greeting.txt.tmpl": "hello from " + version + "\n",
			"greet/hooks/stamp":                 "#!/bin/sh\ncat >/dev/null\necho " + version + " > stamp.txt\n",
		}
		for name, content := range files {
			full := filepath.Join(dir, name)
			os.MkdirAll(filepath.Dir(full), 0o755)
			if err := os.WriteFile(full, []byte(content), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	git("init", "-q")
	write("1.0.0")
	git("add", ".")
	git("commit", "-q", "-m", "1.0.0")
	return "file://" + dir, func(version string) {
		write(version)
		git("commit", "-q", "-am", version)
	}
}

func TestGenWithABlueprintFromGit(t *testing.T) {
	t.Setenv("ESQUADRO_CACHE", t.TempDir())
	url, release := greetRepo(t)
	project := t.TempDir()
	writeFile(t, project, "esquadro/app.esq", "app shop {\n  module \"example.com/shop\"\n  blueprint \""+url+"//greet@main\"\n}\n")
	read := func(name string) string {
		data, _ := os.ReadFile(filepath.Join(project, name))
		return string(data)
	}
	locked := func() workspace.LockedBlueprint {
		t.Helper()
		lock, err := workspace.ReadLock(project)
		if err != nil || len(lock.Blueprints) != 1 {
			t.Fatalf("lock: %+v, %v", lock, err)
		}
		return lock.Blueprints[0]
	}
	answer := func(isTerminal bool, text string) {
		interactive = func() bool { return isTerminal }
		stdin = strings.NewReader(text)
	}
	t.Cleanup(func() { answer(false, "") })

	// Hooks from git do not run until the user allows them.
	answer(false, "")
	if code, _, errOut := gen(t, project); code != 1 || !strings.Contains(errOut, "esquadro gen -trust") {
		t.Fatalf("without trust: exit %d, %s", code, errOut)
	}
	if code, out, errOut := gen(t, "-trust", project); code != 0 {
		t.Fatalf("with -trust: exit %d\n%s%s", code, out, errOut)
	}
	first := locked()
	if read("greeting.txt") != "hello from 1.0.0\n" || read("stamp.txt") != "1.0.0\n" || first.Commit == "" || !first.Trusted {
		t.Fatalf("first generation: %q %q %+v", read("greeting.txt"), read("stamp.txt"), first)
	}

	// The branch moves, but the lockfile pins the commit, which is trusted.
	release("2.0.0")
	if code, out, errOut := gen(t, project); code != 0 || read("greeting.txt") != "hello from 1.0.0\n" || locked().Commit != first.Commit {
		t.Fatalf("pinned: exit %d\n%s%s", code, out, errOut)
	}

	// -update moves to the new commit, whose hooks are asked about again.
	answer(true, "n\n")
	if code, _, errOut := gen(t, "-update", project); code != 1 || !strings.Contains(errOut, "runs programs on this machine") {
		t.Fatalf("refused: exit %d, %s", code, errOut)
	}
	answer(true, "y\n")
	if code, out, errOut := gen(t, "-update", project); code != 0 {
		t.Fatalf("allowed: exit %d\n%s%s", code, out, errOut)
	}
	if second := locked(); read("greeting.txt") != "hello from 2.0.0\n" || read("stamp.txt") != "2.0.0\n" || second.Commit == first.Commit || !second.Trusted {
		t.Fatalf("updated: %q %+v", read("greeting.txt"), second)
	}
}
