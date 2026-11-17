package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/workspace"
)

func TestBlueprintListShowsEveryBuiltinAndWhatTheAppUses(t *testing.T) {
	// Outside a project: every built-in one, none marked.
	code, out, errOut := esq(t, "blueprint", "list", t.TempDir())
	if code != 0 {
		t.Fatal(errOut)
	}
	names, _ := filepath.Glob("../../blueprints/*/blueprint.esq")
	if len(names) != len(blueprintOrder) {
		t.Fatalf("%d manifests, %d in blueprintOrder", len(names), len(blueprintOrder))
	}
	for _, name := range blueprintOrder {
		if !strings.Contains(out, "    "+name+" ") {
			t.Errorf("%s is missing or marked:\n%s", name, out)
		}
		if blueprintGroups[name] == "" {
			t.Errorf("%s has no group", name)
		}
	}
	if strings.Contains(out, "* ") {
		t.Fatalf("marked outside a project:\n%s", out)
	}

	// In a project: what it uses, and its own blueprints.
	dir := demoProject(t)
	if code, out, errOut = esq(t, "blueprint", "list", dir); code != 0 {
		t.Fatal(errOut)
	}
	if !strings.Contains(out, "The project's own:\n  * demo ") || !strings.Contains(out, "    app ") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestBlueprintInfo(t *testing.T) {
	code, out, errOut := esq(t, "blueprint", "info", "worker", t.TempDir())
	if code != 0 {
		t.Fatal(errOut)
	}
	for _, want := range []string{
		"worker 0.1.0 (built-in)",
		"Requires:   app\nWorks with: http",
		"  worker.processes\n",
		"  cmd/<app>/processes.go  (yours: written once, then left alone)\n",
		"  internal/platform/worker/worker.go\n",
		"Adds code to the slots app.commands.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for line := range strings.SplitSeq(out, "\n") {
		if len(line) > 78 && !strings.Contains(line, "/") {
			t.Errorf("line longer than 78: %q", line)
		}
	}
	if code, out, _ = esq(t, "blueprint", "info", "rate_limit", t.TempDir()); code != 0 || !strings.Contains(out, "  per_minute int, default 120\n") {
		t.Fatalf("options:\n%s", out)
	}
	if code, out, _ = esq(t, "blueprint", "info", "demo", demoProject(t)); code != 0 || !strings.Contains(out, "demo 0.1.0") {
		t.Fatalf("the project's own:\n%s", out)
	}
	if code, _, errOut = esq(t, "blueprint", "info", "nope", t.TempDir()); code != 1 || !strings.Contains(errOut, "there is no blueprint nope") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestBlueprintNewThenTest(t *testing.T) {
	dir, appEsq := smallProject(t, "")
	code, out, errOut := esq(t, "blueprint", "new", "billing", dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(read(t, appEsq), "  blueprint \"./blueprints/billing\" {\n    // greeting \"Hello\"\n  }\n") {
		t.Fatalf("app.esq:\n%s", read(t, appEsq))
	}
	bp := filepath.Join(dir, "esquadro", "blueprints", "billing")
	for _, f := range []string{"testdata/expected/internal/billing/billing.go", "testdata/expected/internal/billing/product.go", "testdata/expected/internal/billing/notes.md"} {
		if _, err := os.Stat(filepath.Join(bp, f)); err != nil {
			t.Errorf("expected file: %v", err)
		}
	}
	if got := read(t, filepath.Join(bp, "testdata/expected/internal/billing/billing.go")); !strings.Contains(got, `const Greeting = "Hi"`) || !strings.Contains(got, `Greeting + ", Product",`) {
		t.Fatalf("billing.go:\n%s", got)
	}
	if code, out, errOut = esq(t, "blueprint", "test", bp); code != 0 || !strings.Contains(out, "billing: ok, 4 files as expected") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}

	// A change to a template shows; -update accepts it.
	tmpl := filepath.Join(bp, "templates", "internal", "billing", "billing.go.tmpl")
	os.WriteFile(tmpl, []byte(strings.Replace(read(t, tmpl), "is the greeting option", "is the greeting", 1)), 0o644)
	if code, out, errOut = esq(t, "blueprint", "test", bp); code != 1 || !strings.Contains(out, "-// Greeting is the greeting option of the blueprint.") || !strings.Contains(errOut, "FAIL, 1 difference") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if code, _, errOut = esq(t, "blueprint", "test", "-update", bp); code != 0 {
		t.Fatal(errOut)
	}
	if code, _, errOut = esq(t, "blueprint", "test", bp); code != 0 {
		t.Fatal(errOut)
	}

	// The project generates with it, and checks.
	if code, out, errOut = esq(t, "check", dir); code != 0 {
		t.Fatal(errOut)
	}
	if code, _, errOut = esq(t, "blueprint", "new", "billing", dir); code != 1 || !strings.Contains(errOut, "already exists") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if code, _, errOut = esq(t, "blueprint", "new", "Billing", dir); code != 2 || !strings.Contains(errOut, "is not a blueprint name") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if code, _, errOut = esq(t, "blueprint", "new", "billing", t.TempDir()); code != 1 || !strings.Contains(errOut, "is not a project") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestBlueprintUpdateMovesOnlyTheNamedOnes(t *testing.T) {
	t.Setenv("ESQUADRO_CACHE", t.TempDir())
	url, release := greetRepo(t)
	project := t.TempDir()
	writeFile(t, project, "esquadro/app.esq", "app shop {\n  module \"example.com/shop\"\n  blueprint \""+url+"//greet@main\"\n}\n")
	if code, _, errOut := esq(t, "gen", "-trust", project); code != 0 {
		t.Fatal(errOut)
	}
	release("2.0.0")
	if code, out, _ := esq(t, "gen", project); code != 0 || strings.Contains(read(t, filepath.Join(project, "greeting.txt")), "2.0.0") {
		t.Fatalf("gen moved the blueprint: %s", out)
	}
	if code, out, errOut := esq(t, "blueprint", "update", "nope", project); code != 1 || !strings.Contains(errOut, "those are greet") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	code, out, errOut := esq(t, "blueprint", "update", "-trust", "greet", project)
	if code != 0 || !strings.Contains(out, "greet: ") || !strings.Contains(out, " -> ") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if got := read(t, filepath.Join(project, "greeting.txt")); got != "hello from 2.0.0\n" {
		t.Fatalf("greeting.txt: %q", got)
	}
	if code, out, _ = esq(t, "blueprint", "update", "-trust", project); code != 0 || !strings.Contains(out, "greet: already at the latest commit") {
		t.Fatalf("again:\n%s", out)
	}
	lock, _ := workspace.ReadLock(project)
	if lock.Blueprints[0].Commit == "" || !lock.Blueprints[0].Trusted {
		t.Fatalf("lock: %+v", lock.Blueprints[0])
	}

	// A project without blueprints from git.
	if code, out, _ = esq(t, "blueprint", "update", demoProject(t)); code != 0 || !strings.Contains(out, "no blueprints from git") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestBlueprintHelp(t *testing.T) {
	code, out, _ := esq(t, "blueprint")
	if code != 0 {
		t.Fatal(code)
	}
	for _, c := range blueprintCommands() {
		if !strings.Contains(out, "  "+c.name+" ") {
			t.Errorf("%s missing from:\n%s", c.name, out)
		}
	}
	if code, _, errOut := esq(t, "blueprint", "nope"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestBlueprintListShowsAPlatformItsOwn(t *testing.T) {
	names, _ := filepath.Glob("../../blueprints/platform/*/blueprint.esq")
	for _, n := range names {
		name := filepath.Base(filepath.Dir(n))
		if !slices.Contains(platformBlueprintOrder, name) || platformGroups[name] == "" {
			t.Errorf("%s is not in platformBlueprintOrder, or has no group", name)
		}
	}
	dir := filepath.Join(t.TempDir(), "acme")
	if code, _, errOut := esq(t, "new", "-starter", "platform", "-no-gen", "-no-git", dir); code != 0 {
		t.Fatal(errOut)
	}
	code, out, errOut := esq(t, "blueprint", "list", dir)
	if code != 0 {
		t.Fatal(errOut)
	}
	if !strings.Contains(out, "  * server ") || !strings.Contains(out, "    git ") || strings.Contains(out, " rest ") {
		t.Fatalf("out:\n%s", out)
	}
}
