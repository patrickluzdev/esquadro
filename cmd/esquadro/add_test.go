package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// esq runs esquadro and returns its exit code and output.
func esq(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// smallProject is a project as esquadro new writes it without a starter,
// with a comment of the user's.
func smallProject(t *testing.T, blueprints string) (dir, appEsq string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "shop")
	args := []string{"new", "-no-gen", "-no-git", "-module", "example.com/shop", dir}
	if blueprints != "" {
		args = append(args, "-blueprints", blueprints)
	}
	if code, _, errOut := esq(t, args...); code != 0 {
		t.Fatal(errOut)
	}
	appEsq = filepath.Join(dir, "esquadro", "app.esq")
	src, _ := os.ReadFile(appEsq)
	mine := strings.Replace(string(src), "  blueprint app\n", "  // mine: keep this\n  blueprint app\n", 1)
	os.WriteFile(appEsq, []byte(mine), 0o644)
	return dir, appEsq
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAddWritesTheBlueprintsWithWhatTheyRequire(t *testing.T) {
	dir, appEsq := smallProject(t, "")
	code, out, errOut := esq(t, "add", "-no-gen", "rate_limit", dir)
	if code != 0 {
		t.Fatal(errOut)
	}
	if !strings.Contains(out, "added rate_limit (and http, which it requires) to app.esq") {
		t.Fatalf("out: %s", out)
	}
	want := "  // mine: keep this\n  blueprint app\n  blueprint env\n" +
		"  blueprint http {\n    // port 8080\n    // trust_proxy false\n  }\n" +
		"  blueprint rate_limit {\n    // per_minute 120\n  }\n\n" +
		"  // Add more with esquadro add <blueprint>; see them with esquadro blueprint list.\n}\n"
	if got := read(t, appEsq); !strings.HasSuffix(got, want) {
		t.Fatalf("app.esq:\n%s", got)
	}
	if code, _, errOut := esq(t, "check", dir); code != 0 {
		t.Fatal(errOut)
	}
	if code, out, _ := esq(t, "fmt", "-check", filepath.Join(dir, "esquadro")); code != 0 {
		t.Fatalf("not formatted: %s", out)
	}

	// Again: nothing to do.
	before := read(t, appEsq)
	if code, out, _ := esq(t, "add", "-no-gen", "http", dir); code != 0 || !strings.Contains(out, "http is already in the app") {
		t.Fatalf("exit %d: %s", code, out)
	}
	if read(t, appEsq) != before {
		t.Fatal("app.esq changed")
	}
}

func TestAddGoesBeforeGit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-no-gen", "-module", "example.com/shop", dir); code != 0 {
		t.Fatal(errOut)
	}
	if code, _, errOut := esq(t, "add", "-no-gen", "rate_limit", dir); code != 0 {
		t.Fatal(errOut)
	}
	got := read(t, filepath.Join(dir, "esquadro", "app.esq"))
	if !strings.Contains(got, "  blueprint env\n  blueprint http {") || !strings.Contains(got, "  }\n  blueprint git {") {
		t.Fatalf("app.esq:\n%s", got)
	}
}

func TestAddRefuses(t *testing.T) {
	dir, appEsq := smallProject(t, "")
	before := read(t, appEsq)
	for _, tc := range []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"add", "-no-gen", "nope", dir}, 1, "there is no blueprint nope"},
		{[]string{"add", "-no-gen"}, 2, "expected the blueprints"},
		{[]string{"add", "-no-gen", "./blueprints/missing", dir}, 1, "there is no blueprint ./blueprints/missing"},
	} {
		if code, _, errOut := esq(t, tc.args...); code != tc.code || !strings.Contains(errOut, tc.msg) {
			t.Errorf("%v: exit %d, %q", tc.args, code, errOut)
		}
	}
	if read(t, appEsq) != before {
		t.Fatal("app.esq changed")
	}
}

func TestAddAndRemoveALocalBlueprint(t *testing.T) {
	dir := demoProject(t)
	extra := filepath.Join(dir, "esquadro", "blueprints", "extra")
	os.MkdirAll(filepath.Join(extra, "templates"), 0o755)
	os.WriteFile(filepath.Join(extra, "blueprint.esq"), []byte("blueprint extra {\n  version \"0.1.0\"\n  /// A title.\n  option title string default(\"Shop\")\n}\n"), 0o644)
	os.WriteFile(filepath.Join(extra, "templates", "EXTRA.md.tmpl"), []byte("{{.Options.title}}\n"), 0o644)
	os.WriteFile(filepath.Join(extra, "templates", "NOTES.md.once.tmpl"), []byte("yours\n"), 0o644)

	if code, out, errOut := esq(t, "add", "./blueprints/extra", dir); code != 0 || !strings.Contains(out, "create    EXTRA.md") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	esqFile := filepath.Join(dir, "esquadro", "shop.esq")
	if got := read(t, esqFile); !strings.Contains(got, "  blueprint \"./blueprints/extra\" {\n    // title \"Shop\"\n  }\n") {
		t.Fatalf("shop.esq:\n%s", got)
	}

	if code, out, errOut := esq(t, "remove", "extra", dir); code != 0 || !strings.Contains(out, "delete    EXTRA.md") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if strings.Contains(read(t, esqFile), "extra") {
		t.Fatal("the blueprint is still in shop.esq")
	}
	if _, err := os.Stat(filepath.Join(dir, "EXTRA.md")); !os.IsNotExist(err) {
		t.Fatal("the generated file stayed")
	}
	if read(t, filepath.Join(dir, "NOTES.md")) != "yours\n" {
		t.Fatal("the file written once for the user went")
	}
}

func TestRemoveDropsTheCommentOfAnEmptyGroup(t *testing.T) {
	dir, appEsq := smallProject(t, "app,http,compression")
	if code, out, errOut := esq(t, "remove", "-no-gen", "compression", dir); code != 0 || !strings.Contains(out, "removed compression from app.esq") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	want := "  // the program, its modules and how it runs\n  // mine: keep this\n  blueprint app\n  blueprint http\n}\n"
	if got := read(t, appEsq); !strings.HasSuffix(got, want) {
		t.Fatalf("app.esq:\n%s", got)
	}
	if code, out, _ := esq(t, "fmt", "-check", filepath.Join(dir, "esquadro")); code != 0 {
		t.Fatalf("not formatted: %s", out)
	}
}

func TestRemoveRefuses(t *testing.T) {
	dir, appEsq := smallProject(t, "app,domain,postgres,migrations")
	before := read(t, appEsq)
	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{"remove", "-no-gen", "postgres", dir}, "migrations requires postgres"},
		{[]string{"remove", "-no-gen", "compression", dir}, "the app has no blueprint compression"},
	} {
		if code, _, errOut := esq(t, tc.args...); code != 1 || !strings.Contains(errOut, tc.msg) {
			t.Errorf("%v: exit %d, %q", tc.args, code, errOut)
		}
	}
	if read(t, appEsq) != before {
		t.Fatal("app.esq changed")
	}
	if code, out, errOut := esq(t, "remove", "-no-gen", "migrations", dir); code != 0 ||
		!strings.Contains(out, "no blueprint requires postgres any more") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
}
