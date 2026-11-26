package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fieldReadProject is the api starter with the products' price read only by
// the staff: changed rewrites catalog.esq first.
func fieldReadProject(t *testing.T, change func(string) string) (string, int, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	catalog := filepath.Join(dir, "esquadro", "catalog.esq")
	src := strings.Replace(read(t, catalog),
		"  price       decimal required min(0) @store(filter, sort)",
		"  price       decimal required min(0) @store(filter, sort) @security(read [user.admin, user.staff])", 1)
	if change != nil {
		src = change(src)
	}
	os.WriteFile(catalog, []byte(src), 0o644)
	code, out, errOut := esq(t, "gen", dir)
	return dir, code, out + errOut
}

// TestFieldReadOnlyBySome generates a field only the staff may read and runs
// the project's own access tests: the others get products without it, and
// cannot filter or sort by it.
func TestFieldReadOnlyBySome(t *testing.T) {
	if testing.Short() {
		t.Skip("generates the api starter, which needs sqlc or Docker")
	}
	dir, code, out := fieldReadProject(t, nil)
	if code != 0 {
		t.Fatal(out)
	}
	http := read(t, filepath.Join(dir, "internal", "catalog", "http.go"))
	for _, want := range []string{
		`if !security.Allows(ctx, "user.admin", "user.staff") {`,
		"edge.Unset(&a.Price)",
		"params.PriceMin.IsSet()",
		`q.Sort == "-price"`,
	} {
		if !strings.Contains(http, want) {
			t.Errorf("http.go lacks %s", want)
		}
	}
	if contract := read(t, filepath.Join(dir, "api", "openapi.yaml")); !strings.Contains(contract, "`price` comes only to user.admin, user.staff") {
		t.Error("the contract does not say who reads the price")
	}
	cmd := exec.Command("go", "test", "./cmd/shop/", "-run", "TestAPIAccess")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the API's access tests: %v\n%s", err, out)
	}
}

func TestFieldReadChecksTheModel(t *testing.T) {
	if testing.Short() {
		t.Skip("generates the api starter, which needs sqlc or Docker")
	}
	for _, tc := range []struct{ name, from, to, want string }{
		{"owner", "@security(read [user.admin, user.staff])", "@security(read owner)", "owner is for records"},
		{"unknown realm", "@security(read [user.admin, user.staff])", "@security(read client)", "client is not public nor a realm"},
		{"unknown role", "@security(read [user.admin, user.staff])", "@security(read user.boss)", "boss is not a role of user"},
		{"search", "  description text    @store(search)", "  description text    @store(search) @security(read user.admin)", "cannot be in the search"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, code, out := fieldReadProject(t, func(src string) string { return strings.Replace(src, tc.from, tc.to, 1) })
			if code == 0 || !strings.Contains(out, tc.want) {
				t.Fatalf("gen: code %d, want %q in:\n%s", code, tc.want, out)
			}
		})
	}
}
