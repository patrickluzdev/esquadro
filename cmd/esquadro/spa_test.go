package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// spaProject is the api starter with the workspace and an admin spa, whose
// resources are the catalog's entities; change rewrites app.esq first.
func spaProject(t *testing.T, change func(app string) string) (string, int, string) {
	t.Helper()
	if testing.Short() {
		// Generating the api starter runs its hooks, sqlc among them.
		t.Skip("generates the api starter, which needs sqlc or Docker")
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	app := strings.Replace(read(t, appEsq), "  blueprint api_docs\n",
		"  blueprint api_docs\n  blueprint client_ts {\n    scope \"acme\"\n  }\n  blueprint workspace\n  blueprint ui\n  blueprint spa admin {\n    roles [admin, staff]\n    locale pt_br\n  }\n", 1)
	if change != nil {
		app = change(app)
	}
	os.WriteFile(appEsq, []byte(app), 0o644)
	catalogEsq := filepath.Join(dir, "esquadro", "catalog.esq")
	catalog := strings.Replace(read(t, catalogEsq), "  @security(read public, write [user.admin, user.staff])",
		"  @security(read public, write [user.admin, user.staff])\n  @ui(label \"Produtos\", icon package)\n  @admin(resource)", 1)
	catalog = strings.Replace(catalog, "  name        string  required", "  name        string  required @ui(label \"Nome\")", 1)
	catalog = strings.Replace(catalog, "  ACTIVE\n", "  ACTIVE @ui(label \"Ativo\")\n", 1)
	catalog += "\nreport ProductsInAll {\n  products int required @ui(label \"Produtos\")\n  @security(read user)\n  @admin(widget stat, y products)\n}\n"
	catalog = strings.Replace(catalog, "  @security(read public, write user.admin)", "  @security(read public, write user.admin)\n  @admin(resource)", 1)
	os.WriteFile(catalogEsq, []byte(catalog), 0o644)
	code, _, errOut := esq(t, "gen", dir)
	return dir, code, errOut
}

func TestSPAGeneratesAPanel(t *testing.T) {
	dir, code, errOut := spaProject(t, nil)
	if code != 0 {
		t.Fatal(errOut)
	}
	// The CI checks the code out before any step reads it (the client's
	// type check, the frontends' packages).
	ci := read(t, filepath.Join(dir, ".github", "workflows", "ci.yml"))
	test := ci[strings.Index(ci, "\n  test:"):]
	if checkout, reads := strings.Index(test, "actions/checkout"), strings.Index(test, "working-directory:"); checkout < 0 || reads >= 0 && reads < checkout {
		t.Errorf("the CI reads the code before checking it out:\n%s", test)
	}
	if checkout, npm := strings.Index(test, "actions/checkout"), strings.Index(test, "npm "); npm >= 0 && npm < checkout {
		t.Errorf("the CI installs packages before checking the code out:\n%s", test)
	}
	web := filepath.Join(dir, "web", "admin")
	for path, want := range map[string]string{
		"src/esquadro/resources.gen.ts":     `label: "Produtos"`,
		"src/esquadro/config.ts":            `export const roles: readonly string[] = ["admin", "staff"]`,
		"src/resources/products.tsx":        "f.created_at],",
		"src/pages/_app/products/index.tsx": `createFileRoute("/_app/products/")`,
		"src/esquadro/menu.gen.ts":          "icons.Package",
		"src/esquadro/icons.gen.ts":         "Package: lucide.Package,",
		"src/icons.tsx":                     "export const icons: IconOverrides = {}",
		"src/esquadro/kit/list.tsx":         `} from "../icons.gen.ts"`,
		"src/esquadro/dashboard.gen.ts":     `kind: "stat"`,
		"src/components/site-header.tsx":    "<CommandPalette />",
		"src/resources/categories.tsx":      `relation(products, "category_id")`,
		"src/components/app-sidebar.tsx":    `collapsible="icon"`,
		"src/components/ui/select.tsx":      "SelectPrimitive",
		"components.json":                   `"style": "new-york"`,
		"embed.go":                          "package admin",
		// The team's page in pt_br: roles by their words, counts by theirs,
		// the confirmation of an email sent again, not too soon.
		"src/esquadro/kit/messages.ts":        `accountsCount_one: "{count} conta"`,
		"src/esquadro/kit/i18n.ts":            "export function roleLabel(",
		"src/esquadro/kit/accounts-admin.tsx": "accounts.resendVerification(a.id, verifyRedirect())",
		"src/esquadro/kit/errors.ts":          `if (e.code === "too_soon") return t("tooSoon")`,
		// A new password's page shows the realm's minimum.
		"src/esquadro/kit/pages/reset-password.tsx": `t("passwordRule", { min: passwordPolicy.min })`,
		"src/components/user-menu.tsx":              "session?.roles.map(roleLabel)",
		"src/messages.ts":                           `// role_order_desk: "Balcão de pedidos",`,
		"../../clients/ts/src/identity.ts":          "resendVerification: async (id: string, redirect?: string)",
	} {
		if got := read(t, filepath.Join(web, path)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", path, want, got)
		}
	}
	if words := read(t, filepath.Join(web, "src/esquadro/kit/messages.ts")); !strings.Contains(words, `role_admin: "Administração",`) || !strings.Contains(words, `role_admin: "Admin",`) {
		t.Error("the common roles have no words of their own")
	}
	// A password refused says why, with the realm's policy, in pt_br.
	if config := read(t, filepath.Join(web, "src/esquadro/config.ts")); !strings.Contains(config, `export const passwordPolicy: { min: number; max: number } = { min: 12, max: 128 }`) {
		t.Errorf("config.ts has no password policy:\n%s", config)
	}
	if words := read(t, filepath.Join(web, "src/esquadro/kit/messages.ts")); !strings.Contains(words, `passwordTooShort: "A senha precisa ter pelo menos {min} caracteres.",`) {
		t.Error("a password too short has no words in pt_br")
	}
	if gen := read(t, filepath.Join(web, "src/esquadro/resources.gen.ts")); !strings.Contains(gen, `name: { name: "name", label: "Nome"`) || !strings.Contains(gen, `labels: { ACTIVE: "Ativo", INACTIVE: "INACTIVE" }`) || !strings.Contains(gen, `default: "ACTIVE"`) || !strings.Contains(gen, "categories:") {
		t.Errorf("resources.gen.ts:\n%s", gen)
	}
	if cmd := read(t, filepath.Join(dir, "cmd", "shop", "security_command.go")); !strings.Contains(cmd, `"user": {page: "/admin/reset-password"},`) {
		t.Error("a password's link does not open the panel's page by default")
	}
	if pkg := read(t, filepath.Join(dir, "package.json")); !strings.Contains(pkg, `"web/admin"`) || !strings.Contains(pkg, `"clients/ts"`) {
		t.Errorf("package.json:\n%s", pkg)
	}
	if routes := read(t, filepath.Join(dir, "cmd", "shop", "routes_gen.go")); !strings.Contains(routes, `web.Mount(mux, admin.Files, "admin", "", "/admin")`) {
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
		"idle":      {func(s string) string { return strings.Replace(s, "locale pt_br", "idle_minutes 1", 1) }, "idle_minutes 1 is too short"},
		"workspace": {func(s string) string { return strings.Replace(s, "  blueprint workspace\n", "", 1) }, "requires workspace"},
		"no name":   {func(s string) string { return strings.Replace(s, "blueprint spa admin", "blueprint spa", 1) }, "is used by named instances"},
		"reserved":  {func(s string) string { return strings.Replace(s, "locale pt_br", "path \"/api\"", 1) }, "where the app's own routes are"},
		"domain":    {func(s string) string { return strings.Replace(s, "locale pt_br", "domain \"admin\"", 1) }, "domain admin is not a domain name"},
		"twice": {func(s string) string {
			return strings.Replace(s, "  blueprint ui\n", "  blueprint ui\n  blueprint spa staff {\n    path \"/admin\"\n  }\n", 1)
		}, "spa admin answers where spa staff does (/admin)"},
	} {
		_, code, errOut := spaProject(t, tc.change)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", name, code, tc.want, errOut)
		}
	}
}

func TestSPALabelsFromUIAndItsOwn(t *testing.T) {
	dir, code, errOut := spaProject(t, nil)
	if code != 0 {
		t.Fatal(errOut)
	}
	catalogEsq := filepath.Join(dir, "esquadro", "catalog.esq")
	catalog := strings.Replace(read(t, catalogEsq), `@ui(label "Nome")`, `@ui(label "Nome") @admin(label "Nome do produto")`, 1)
	os.WriteFile(catalogEsq, []byte(catalog), 0o644)
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	got := read(t, filepath.Join(dir, "web", "admin", "src", "esquadro", "resources.gen.ts"))
	for _, want := range []string{`label: "Produtos"`, `label: "Nome do produto"`, `ACTIVE: "Ativo"`, "icons.Package"} {
		if !strings.Contains(got+read(t, filepath.Join(dir, "web", "admin", "src", "esquadro", "menu.gen.ts")), want) {
			t.Errorf("lacks %q", want)
		}
	}
}

// TestSPAAndSiteWorkARelationToItself generates comments that answer each
// other (parent > Comment, replies < Comment) with the panel and a site,
// and builds and tests them.
func TestSPAAndSiteWorkARelationToItself(t *testing.T) {
	dir, code, errOut := spaProject(t, func(app string) string {
		return strings.Replace(app, "  blueprint ui\n", "  blueprint ui\n  blueprint ssr site\n", 1)
	})
	if code != 0 {
		t.Fatal(errOut)
	}
	catalogEsq := filepath.Join(dir, "esquadro", "catalog.esq")
	os.WriteFile(catalogEsq, []byte(read(t, catalogEsq)+`
entity Offer {
  title    string required max(120)
  comments < Comment
  @security(read public, write [user.admin, user.staff])
  @admin(resource)
  @site(page, relations [comments])
}

entity Comment {
  body    text required max(2000)
  offer   > Offer required on_delete(cascade) @store(filter)
  parent  > Comment on_delete(cascade) @store(filter)
  replies < Comment
  author  > Customer required
  @security(read public, write owner)
  @store(soft_delete)
  @admin(resource)
  @site(page, relations [replies])
}
`), 0o644)
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	for path, want := range map[string]string{
		"internal/catalog/queries.sql":            "(NOT sqlc.arg(no_parent)::boolean OR parent_id IS NULL)",
		"internal/catalog/list_comments.go":       "NoParent bool",
		"internal/catalog/trash_comment.go":       "q.LiveCommentByParent(",
		"internal/catalog/http.go":                "q.Filter.NoParent = true",
		"api/openapi.yaml":                        "`parent_id=null` keeps those without one",
		"clients/ts/src/records.ts":               `/** An id, or "null" for those without one. */`,
		"web/admin/src/resources/comments.tsx":    `relation(self, "parent_id")`,
		"web/admin/src/esquadro/resources.gen.ts": `target: "comments", display: "body"`,
		"web/site/comment_gen.go":                 "q.Filter.ParentID = x.ID",
	} {
		if got := read(t, filepath.Join(dir, path)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q", path, want)
		}
	}
	// The second generation migrates to the comments, keyed to their own table.
	migrations, _ := filepath.Glob(filepath.Join(dir, "migrations", "*.sql"))
	if !slices.ContainsFunc(migrations, func(p string) bool {
		return strings.Contains(read(t, p), "FOREIGN KEY (parent_id) REFERENCES public.comments (id) ON DELETE CASCADE")
	}) {
		t.Errorf("no migration keys parent_id to comments: %v", migrations)
	}
	for _, args := range [][]string{{"vet", "./..."}, {"test", "./internal/catalog/", "./web/site/"}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// TestFrontendsOnDomainsOfTheirOwn generates a panel and a site on domains
// of their own: they answer on their aliases too (admin.localhost,
// site.localhost), the realm's links open the panel's domain, which is an
// allowed redirect, and in development where each is.
func TestFrontendsOnDomainsOfTheirOwn(t *testing.T) {
	dir, code, errOut := spaProject(t, func(app string) string {
		app = strings.Replace(app, "  blueprint spa admin {\n", "  blueprint spa admin {\n    domain \"painel.shop.test\"\n", 1)
		return strings.Replace(app, "    locale pt_br\n  }\n", "    locale pt_br\n  }\n  blueprint ssr site {\n    domain \"shop.test\"\n    locale pt_br\n  }\n", 1)
	})
	if code != 0 {
		t.Fatal(errOut)
	}
	for path, want := range map[string][]string{
		"cmd/shop/routes_gen.go": {
			`web.Mount(mux, admin.Files, "admin", "painel.shop.test", "")`,
			`}), "site", site.Domain, site.Path)`,
		},
		"cmd/shop/security_command.go": {`"user": {page: "https://painel.shop.test/reset-password"},`},
		"cmd/shop/dependencies.go": {
			`"painel.shop.test": "http://localhost:5173",`,
			`"shop.test":        security.OnHost(d.config.PublicURL, "site.localhost"),`,
			`[]string{"https://painel.shop.test", "https://shop.test"},`,
		},
		"internal/platform/web/frontend.go":                    {`mux.Handle(Alias(name)+"/", on)`},
		"web/admin/src/esquadro/config.ts":                     {`export const origin: string = "https://painel.shop.test"`},
		"web/admin/src/esquadro/kit/pages/forgot-password.tsx": {`redirect: pageURL("reset-password")`},
		"web/site/account_gen.go":                              {`return "https://" + Domain + strings.TrimPrefix(path, Path)`},
	} {
		got := read(t, filepath.Join(dir, path))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q", path, w)
			}
		}
	}
	for _, args := range [][]string{
		{"vet", "./..."},
		{"test", "./internal/platform/web/", "./web/site/"},
		{"test", "-run", "TestLinksOpenWhereThePageIs|TestOnHost|TestRedirectsToAFrontendsDomain", "./internal/platform/security/"},
	} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}
