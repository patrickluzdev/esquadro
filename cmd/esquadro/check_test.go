package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/hook"
	"github.com/patrickluzdev/esquadro/internal/project"
)

const okProject = `package catalog

app shop {
  module "github.com/example/shop"
  blueprint app
}

entity Product {
  name string required
}
`

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func check(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := run(append([]string{"check"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCheckOK(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "esquadro/shop.esq", okProject)
	writeFile(t, dir, "esquadro/blueprint.esq", "not a project file")
	writeFile(t, dir, "esquadro/sub/ignored.esq", "not read: subdirectories are not part of the project")

	code, out, errOut := check(t)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, errOut)
	}
	if out != "ok: app shop (1 file, 1 entity, 0 enums, 1 blueprint)\n" {
		t.Fatalf("stdout = %q", out)
	}
}

func TestCheckMultipleFilesAndDirArgument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "esquadro/app.esq", "app shop {\n  module \"github.com/example/shop\"\n  blueprint app\n}\n")
	writeFile(t, dir, "esquadro/catalog.esq", "package catalog\n\nentity Product {\n  name string\n  category > Category\n}\n")
	writeFile(t, dir, "esquadro/category.esq", "package catalog\n\nentity Category {\n  name string\n}\n")

	code, out, errOut := check(t, dir)
	if code != 0 || out != "ok: app shop (3 files, 2 entities, 0 enums, 1 blueprint)\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestCheckReportsErrorsWithFileNames(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "esquadro/app.esq", "app shop {\n  module \"github.com/example/shop\"\n  blueprint app\n}\n")
	writeFile(t, dir, "esquadro/catalog.esq", "package catalog\n\nentity Product {\n  name strng\n}\n")

	code, out, errOut := check(t)
	if code != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	for _, want := range []string{`catalog.esq:4:8: error: unknown type "strng"`, `4 | `, `did you mean "string"?`, "1 error"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
}

func TestCheckPrintsWarningsButSucceeds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "esquadro/shop.esq", strings.Replace(okProject, "  blueprint app\n", "", 1))

	code, out, errOut := check(t)
	if code != 0 || !strings.HasPrefix(out, "ok:") {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	if !strings.Contains(errOut, "warning: app shop uses no blueprints") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestCheckNoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if code, _, errOut := check(t); code != 1 || !strings.Contains(errOut, "no esquadro/ folder with .esq files in .") {
		t.Fatalf("empty dir: exit %d, %q", code, errOut)
	}
	writeFile(t, dir, "esquadro/notes.txt", "")
	if code, _, errOut := check(t); code != 1 || !strings.Contains(errOut, "no .esq files in esquadro") {
		t.Fatalf("empty esquadro/: exit %d, %q", code, errOut)
	}
	if code, _, errOut := check(t, "missing"); code != 1 || !strings.Contains(errOut, "missing") {
		t.Fatalf("missing dir: exit %d, %q", code, errOut)
	}
	writeFile(t, dir, "esquadro/shop.esq", okProject)
	if code, _, errOut := check(t, "esquadro/shop.esq"); code != 1 || !strings.Contains(errOut, "shop.esq is not a directory") {
		t.Fatalf("file argument: exit %d, %q", code, errOut)
	}
}

func TestCheckPointsOldProjectsToEsquadroFolder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", okProject)
	if code, _, errOut := check(t); code != 1 || !strings.Contains(errOut, "the .esq files go in esquadro/ now") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
}

func TestCheckBadArguments(t *testing.T) {
	if code, _, _ := check(t, "-nope"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if code, _, _ := check(t, "a", "b"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestCheckValidatesBlueprints(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "esquadro/shop.esq", `package catalog

app shop {
  module "github.com/example/shop"
  blueprint postgres {
    pool_max "ten"
  }
}

entity Product {
  name string @postgres(colum product_name)
}
`)
	code, _, errOut := check(t)
	if code != 1 {
		t.Fatalf("exit %d, stderr:\n%s", code, errOut)
	}
	for _, want := range []string{
		"blueprint postgres requires app",
		"option pool_max of blueprint postgres: expected a whole number",
		`@postgres has no key "colum" on fields`,
		`did you mean "column"?`,
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
}

func TestEveryBuiltinHookIsRegistered(t *testing.T) {
	l := &blueprint.Loader{Builtin: blueprints.FS}
	for _, name := range l.BuiltinNames() {
		b, diags := l.Load(&project.BlueprintRef{Ref: name, Source: project.SourceBuiltin})
		if b == nil {
			continue // a hook file is missing: the loader reports it below
		}
		for _, d := range diags {
			t.Errorf("built-in %s: %v", name, d)
		}
		for _, h := range b.Manifest.Hooks {
			if builtinHooks[hook.Key(name, h.Path)] == nil {
				t.Errorf("built-in blueprint %s declares hook %s, which is not in builtinHooks", name, h.Path)
			}
		}
	}
}
