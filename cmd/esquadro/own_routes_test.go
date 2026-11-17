package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOwnRoutesAreOptionalAndStayDeleted generates the app's own routes,
// leaves them out with own_routes false, and deletes them: the app builds
// every time, and esquadro gen never brings a deleted file back.
func TestOwnRoutesAreOptionalAndStayDeleted(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	build := func(dir string) {
		t.Helper()
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
	}
	file := func(dir string) string { return filepath.Join(dir, "cmd", "shop", "routes.go") }

	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-blueprints", "app,env,http", "-module", "example.com/shop", "-no-git", dir); code != 0 {
		t.Fatal(errOut)
	}
	src, err := os.ReadFile(file(dir))
	if err != nil || !strings.Contains(string(src), `"GET /robots.txt"`) {
		t.Fatalf("routes.go: %v\n%s", err, src)
	}
	build(dir)

	// Deleted, it builds, and stays deleted.
	if err := os.Remove(file(dir)); err != nil {
		t.Fatal(err)
	}
	build(dir)
	code, out, errOut := esq(t, "gen", dir)
	if code != 0 {
		t.Fatal(errOut)
	}
	if _, err := os.Stat(file(dir)); err == nil || !strings.Contains(out+errOut, "you deleted it") {
		t.Fatalf("esquadro gen brought it back, or did not say so:\n%s%s", out, errOut)
	}

	// Left out by the option.
	off := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-blueprints", "app,env,http", "-module", "example.com/shop", "-no-git", "-no-gen", off); code != 0 {
		t.Fatal(errOut)
	}
	model := filepath.Join(off, "esquadro", "app.esq")
	app, _ := os.ReadFile(model)
	os.WriteFile(model, []byte(strings.Replace(string(app), "  blueprint http\n", "  blueprint http {\n    own_routes false\n  }\n", 1)), 0o644)
	if code, _, errOut := esq(t, "gen", off); code != 0 {
		t.Fatal(errOut)
	}
	if _, err := os.Stat(file(off)); err == nil {
		t.Fatal("own_routes false still writes routes.go")
	}
	build(off)
}
