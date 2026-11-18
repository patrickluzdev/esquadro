//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCanaryUpgrades generates a project of the model of a real one with the
// last release of esquadro, changes it as its team would, and upgrades it
// with this esquadro: it builds and passes its tests, nothing of the team's
// is lost, and the upgrade leaves no conflict.
func TestCanaryUpgrades(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatal("Docker is not available")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tag := strings.TrimSpace(gitOut(t, repo, "describe", "--tags", "--abbrev=0", "--match", "v*"))
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(cache, "esquadro-canary-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })

	// The last release, built from its tag.
	src := filepath.Join(root, "release")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := exec.Command("sh", "-c", "git archive "+tag+" | tar -x -C "+src)
	archive.Dir = repo
	if out, err := archive.CombinedOutput(); err != nil {
		t.Fatalf("git archive %s: %v\n%s", tag, err, out)
	}
	old := filepath.Join(root, "esquadro-"+tag)
	build := exec.Command("go", "build", "-ldflags", "-X github.com/patrickluzdev/esquadro/internal/version.Version="+tag, "-o", old, "./cmd/esquadro")
	build.Dir = src
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building esquadro %s: %v\n%s", tag, err, out)
	}

	// The project, as that release generates it.
	dir := filepath.Join(root, "promo")
	e := &lifecycle{t: t, dir: root}
	e.run(nil, old, "new", "-starter", "api", "-module", "example.com/promo", "-no-git", "-no-gen", "promo")
	e.dir = dir
	read := func(path string) string { return e.read(filepath.Join(dir, path)) }
	write := func(path, content string) { e.write(filepath.Join(dir, path), content) }
	canary := filepath.Join(repo, "cmd", "esquadro", "testdata", "canary", "promoduro")
	for _, name := range []string{"catalog.esq", "accounts.esq"} {
		os.Remove(filepath.Join(dir, "esquadro", name))
	}
	models, _ := filepath.Glob(filepath.Join(canary, "*.esq"))
	for _, m := range models {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join("esquadro", filepath.Base(m)), string(data))
	}
	e.run(nil, old, "gen", ".")
	e.run(nil, "go", "build", "./...")

	// What the team changes: a use case of its own, a job's work, a task,
	// the README.
	e.run(nil, "cp", "-R", filepath.Join(canary, "yours")+"/.", ".")
	work := read("internal/deals/fetch_offers_work.go")
	write("internal/deals/fetch_offers_work.go", strings.Replace(work, "\t_, _ = ctx, args", "\t// Ask the affiliate programs for offers.\n\t_, _ = ctx, args", 1))
	write("Taskfile.yml", read("Taskfile.yml")+"\ntasks:\n  fetch:\n    desc: Fetch offers now\n    cmds:\n      - go run ./cmd/promo run\n")
	write("README.md", read("README.md")+"\n## Promo do Duro\n\nThe groups, the site and the links.\n")
	e.run(nil, "go", "build", "./...")

	// Upgraded by this esquadro.
	out := e.esquadro(0, "upgrade", dir)
	if marked := e.run(nil, "sh", "-c", "grep -rl --exclude-dir=esquadro '^<<<<<<< yours' . || true"); marked != "" {
		t.Fatalf("the upgrade left conflicts in %s\n%s", marked, out)
	}
	e.esquadro(0, "gen", "-check", dir)
	if !strings.Contains(read("internal/deals/approve.go"), "ApproveOffer") ||
		!strings.Contains(read("internal/deals/fetch_offers_work.go"), "Ask the affiliate programs") ||
		!strings.Contains(read("Taskfile.yml"), "fetch:") ||
		!strings.Contains(read("README.md"), "## Promo do Duro") {
		t.Fatal("the upgrade lost a change of the team's")
	}
	e.run(nil, "go", "vet", "./...")

	// A feature of this esquadro adopted: money, in the app's currency.
	deals := read("esquadro/deals.esq")
	write("esquadro/deals.esq", regexp.MustCompile(`\bdecimal\b`).ReplaceAllString(deals, "money"))
	write("esquadro/app.esq", strings.Replace(read("esquadro/app.esq"), `  module "example.com/promo"`, "  module \"example.com/promo\"\n  currency \"BRL\"", 1))
	e.esquadro(0, "gen", dir)
	e.run(nil, "go", "vet", "./...")
	e.run(nil, "go", "test", "./...")
	e.esquadro(0, "gen", "-check", dir)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}
