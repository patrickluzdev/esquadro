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

	code, out, errOut := check(t)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, errOut)
	}
	if out != "shop.esq: ok (app shop, 1 entity, 0 enums, 1 blueprint)\n" {
		t.Fatalf("stdout = %q", out)
	}
}

func TestCheckReportsErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", strings.Replace(okProject, "name string", "name strng", 1))

	code, out, errOut := check(t, "-f", "shop.esq")
	if code != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	for _, want := range []string{`shop.esq:7:8: error: unknown type "strng"`, `did you mean "string"?`, "1 error"} {
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
	if code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	if !strings.Contains(errOut, "warning: app shop uses no blueprints") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestCheckFileDiscovery(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	code, _, errOut := check(t)
	if code != 1 || !strings.Contains(errOut, "no .esq file in the current directory") {
		t.Fatalf("empty dir: exit %d, %q", code, errOut)
	}

	writeFile(t, dir, "a.esq", okProject)
	writeFile(t, dir, "b.esq", okProject)
	code, _, errOut = check(t)
	if code != 1 || !strings.Contains(errOut, "found several .esq files (a.esq, b.esq)") {
		t.Fatalf("two files: exit %d, %q", code, errOut)
	}

	code, _, errOut = check(t, "-f", "missing.esq")
	if code != 1 || !strings.Contains(errOut, "missing.esq") {
		t.Fatalf("missing file: exit %d, %q", code, errOut)
	}
}

func TestCheckBadFlag(t *testing.T) {
	if code, _, _ := check(t, "-nope"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if code, _, _ := check(t, "extra"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
