package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPIGivesNumbersAndYesNoTheirDefaults creates records whose number
// and yes/no fields with a default are left out of the request: they take
// their defaults, which may be no empty value (a minimum of 1, true), and
// the API's generated tests, which leave them out, pass.
func TestAPIGivesNumbersAndYesNoTheirDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("generates the api starter, which needs sqlc or Docker")
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	catalog := filepath.Join(dir, "esquadro", "catalog.esq")
	src := read(t, catalog)
	with := strings.Replace(src, "entity Product {\n", "entity Product {\n  stock    int     default(3) min(1)\n  shelf    int64   default(7)\n  featured bool    default(true)\n  weight   decimal default(1.5)\n", 1)
	if with == src {
		t.Fatal("the starter has no entity Product")
	}
	os.WriteFile(catalog, []byte(with), 0o644)
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	http := read(t, filepath.Join(dir, "internal", "catalog", "http.go"))
	for _, want := range []string{"n.Stock = 3", "n.Shelf = 7", "n.Featured = true", `n.Weight = decimal.RequireFromString("1.5")`} {
		if !strings.Contains(http, want) {
			t.Errorf("http.go lacks %q", want)
		}
	}
	cmd := exec.Command("go", "test", "./cmd/shop/", "-run", "TestAPIProducts")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the API's tests of products: %v\n%s", err, out)
	}
}
