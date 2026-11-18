package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// spaProject is the api starter with the workspace and an admin spa, whose
// resources are the catalog's entities; change rewrites app.esq first.
func spaProject(t *testing.T, change func(app string) string) (string, int, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	app := strings.Replace(read(t, appEsq), "  blueprint api_docs\n",
		"  blueprint api_docs\n  blueprint client_ts {\n    scope \"acme\"\n  }\n  blueprint workspace\n  blueprint spa admin {\n    roles [admin, staff]\n    locale pt_br\n  }\n", 1)
	if change != nil {
		app = change(app)
	}
	os.WriteFile(appEsq, []byte(app), 0o644)
	catalogEsq := filepath.Join(dir, "esquadro", "catalog.esq")
	catalog := strings.Replace(read(t, catalogEsq), "  @security(read public, write [user.admin, user.staff])",
		"  @security(read public, write [user.admin, user.staff])\n  @admin(resource, label \"Produtos\", icon package)", 1)
	catalog = strings.Replace(catalog, "  name        string  required", "  name        string  required @admin(label \"Nome\")", 1)
	os.WriteFile(catalogEsq, []byte(catalog), 0o644)
	code, _, errOut := esq(t, "gen", dir)
	return dir, code, errOut
}

func TestSPAGeneratesAPanel(t *testing.T) {
	dir, code, errOut := spaProject(t, nil)
	if code != 0 {
		t.Fatal(errOut)
	}
	web := filepath.Join(dir, "web", "admin")
	for path, want := range map[string]string{
		"src/esquadro/resources.gen.ts":     `label: "Produtos"`,
		"src/esquadro/config.ts":            `export const roles: readonly string[] = ["admin", "staff"]`,
		"src/resources/products.tsx":        "export default resource(meta, {",
		"src/pages/_app/products/index.tsx": `createFileRoute("/_app/products/")`,
		"src/menu.ts":                       "icons.Package",
		"src/components/app-sidebar.tsx":    `collapsible="icon"`,
		"src/components/ui/select.tsx":      "SelectPrimitive",
		"components.json":                   `"style": "new-york"`,
		"embed.go":                          "package admin",
	} {
		if got := read(t, filepath.Join(web, path)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", path, want, got)
		}
	}
	if gen := read(t, filepath.Join(web, "src/esquadro/resources.gen.ts")); !strings.Contains(gen, `name: { name: "name", label: "Nome"`) || strings.Contains(gen, "categories:") {
		t.Errorf("resources.gen.ts:\n%s", gen)
	}
	if pkg := read(t, filepath.Join(dir, "package.json")); !strings.Contains(pkg, `"web/admin"`) || !strings.Contains(pkg, `"clients/ts"`) {
		t.Errorf("package.json:\n%s", pkg)
	}
	if routes := read(t, filepath.Join(dir, "cmd", "shop", "routes_gen.go")); !strings.Contains(routes, `web.Mount(mux, admin.Files, "", "/admin")`) {
		t.Errorf("routes_gen.go:\n%s", routes)
	}
	if ignore := read(t, filepath.Join(dir, ".gitignore")); !strings.Contains(ignore, "node_modules/") {
		t.Errorf(".gitignore:\n%s", ignore)
	}
	for _, args := range [][]string{{"vet", "./..."}, {"test", "./internal/platform/web/"}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func TestSPAChecksItsInstances(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(string) string
		want   string
	}{
		"role":      {func(s string) string { return strings.Replace(s, "roles [admin, staff]", "roles [boss]", 1) }, "boss is not a role of user"},
		"serve":     {func(s string) string { return strings.Replace(s, "locale pt_br", "serve image", 1) }, "serve image is not one esquadro generates yet"},
		"sidebar":   {func(s string) string { return strings.Replace(s, "locale pt_br", "sidebar floating", 1) }, "sidebar floating is not icon, inset or dashboard"},
		"name":      {func(s string) string { return strings.Replace(s, "blueprint spa admin", "blueprint spa Admin", 1) }, "Admin cannot name a frontend"},
		"workspace": {func(s string) string { return strings.Replace(s, "  blueprint workspace\n", "", 1) }, "requires workspace"},
		"no name":   {func(s string) string { return strings.Replace(s, "blueprint spa admin", "blueprint spa", 1) }, "is used by named instances"},
	} {
		_, code, errOut := spaProject(t, tc.change)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", name, code, tc.want, errOut)
		}
	}
}
