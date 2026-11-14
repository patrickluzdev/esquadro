package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const okProject = `app shop {
  module "github.com/example/shop"
  blueprint base
}

entity Product {
  name string required
}
`

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
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
	writeFile(t, dir, "shop.esq", okProject)
	writeFile(t, dir, "blueprint.esq", "not a project file")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub"), "ignored.esq", "not read: subdirectories are not part of the project")

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
	writeFile(t, dir, "app.esq", "app shop {\n  module \"github.com/example/shop\"\n  blueprint base\n}\n")
	writeFile(t, dir, "catalog.esq", "entity Product {\n  name string\n  category -> Category\n}\n")
	writeFile(t, dir, "category.esq", "entity Category {\n  name string\n}\n")

	code, out, errOut := check(t, dir)
	if code != 0 || out != "ok: app shop (3 files, 2 entities, 0 enums, 1 blueprint)\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestCheckReportsErrorsWithFileNames(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "app.esq", "app shop {\n  module \"github.com/example/shop\"\n  blueprint base\n}\n")
	writeFile(t, dir, "catalog.esq", "entity Product {\n  name strng\n}\n")

	code, out, errOut := check(t)
	if code != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	for _, want := range []string{`catalog.esq:2:8: error: unknown type "strng"`, `2 | `, `did you mean "string"?`, "1 error"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
}

func TestCheckPrintsWarningsButSucceeds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", strings.Replace(okProject, "  blueprint base\n", "", 1))

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
	if code, _, errOut := check(t); code != 1 || !strings.Contains(errOut, "no .esq files in .") {
		t.Fatalf("empty dir: exit %d, %q", code, errOut)
	}
	if code, _, errOut := check(t, "missing"); code != 1 || !strings.Contains(errOut, "missing") {
		t.Fatalf("missing dir: exit %d, %q", code, errOut)
	}
	writeFile(t, dir, "shop.esq", okProject)
	if code, _, errOut := check(t, "shop.esq"); code != 1 || !strings.Contains(errOut, "shop.esq is not a directory") {
		t.Fatalf("file argument: exit %d, %q", code, errOut)
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
