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

// TestAPITestsTakeWhatTheRulesNeed gives a business rule that wants more
// than the required fields what it wants, in the requests of the API's
// generated tests (apiGiven, from a test file of the project's), which then
// pass where they were refused.
func TestAPITestsTakeWhatTheRulesNeed(t *testing.T) {
	if testing.Short() {
		t.Skip("generates the api starter, which needs sqlc or Docker")
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	rules := filepath.Join(dir, "internal", "catalog", "product_rules.go")
	src := read(t, rules)
	src = strings.Replace(src, "package catalog\n", "package catalog\n\nimport \"context\"\n", 1)
	src += "\nfunc (ProductRules) BeforeCreate(_ context.Context, _ *Catalog, n *NewProduct) error {\n" +
		"\tif n.Description == \"\" {\n\t\treturn Broken(\"described\", \"A product is described.\", \"description\")\n\t}\n\treturn nil\n}\n"
	os.WriteFile(rules, []byte(src), 0o644)
	test := func() ([]byte, error) {
		cmd := exec.Command("go", "test", "./cmd/shop/", "-run", "TestAPIProducts$")
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if out, err := test(); err == nil || !strings.Contains(string(out), "described") {
		t.Fatalf("the rule did not refuse the tests' product: %v\n%s", err, out)
	}
	os.WriteFile(filepath.Join(dir, "cmd", "shop", "given_test.go"), []byte("package main\n\nimport \"example.com/shop/internal/apigen\"\n\n"+
		"func init() {\n\tapiGiven[\"Product\"] = func(req any, i int) {\n\t\treq.(*apigen.NewProduct).Description = apigen.NewOptString(\"described\")\n\t}\n}\n"), 0o644)
	if out, err := test(); err != nil {
		t.Fatalf("with what the rule needs: %v\n%s", err, out)
	}
}
