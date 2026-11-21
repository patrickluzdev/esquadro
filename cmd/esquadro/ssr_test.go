package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ssrProject is the api starter with a site (ssr shop) beside its panel,
// whose pages are the catalog's; change rewrites app.esq, and model the
// catalog, first.
func ssrProject(t *testing.T, change, model func(string) string) (string, int, string) {
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
		"  blueprint api_docs\n  blueprint workspace\n  blueprint ui\n  blueprint ssr shop {\n    auth \"customer\"\n    locale pt_br\n    name \"Loja\"\n  }\n", 1)
	if change != nil {
		app = change(app)
	}
	os.WriteFile(appEsq, []byte(app), 0o644)
	catalogEsq := filepath.Join(dir, "esquadro", "catalog.esq")
	catalog := read(t, catalogEsq)
	catalog = strings.Replace(catalog, "  @security(read public, write user.admin)",
		"  @security(read public, write user.admin)\n  @ui(label \"Categorias\", singular \"Categoria\")\n  @shop(page, path \"categorias\", relations [products], menu)", 1)
	catalog = strings.Replace(catalog, "  @security(read public, write [user.admin, user.staff])",
		"  @security(read public, write [user.admin, user.staff])\n  @ui(label \"Produtos\", singular \"Produto\")\n  @shop(page, path \"produtos\", description description, schema product, sort \"-price\", menu)", 1)
	if model != nil {
		catalog = model(catalog)
	}
	os.WriteFile(catalogEsq, []byte(catalog), 0o644)
	code, _, errOut := esq(t, "gen", dir)
	return dir, code, errOut
}

func TestSSRGeneratesASite(t *testing.T) {
	dir, code, errOut := ssrProject(t, nil, nil)
	if code != 0 {
		t.Fatal(errOut)
	}
	site := filepath.Join(dir, "web", "shop")
	for path, want := range map[string]string{
		"site_gen.go":                        `Path   = "/shop"`,
		"product_gen.go":                     `m.Read`,
		"category_gen.go":                    `q.Filter.CategoryID = x.ID`,
		"product.templ":                      "templ ProductListPage(v ProductList)",
		"kit.templ":                          `<meta property="og:title"`,
		"layout.templ":                       "templ Layout(p Page)",
		"pages_gen.go":                       "func (s *Site) sitemap(",
		"account_gen.go":                     `identity = "/identity/customer"`,
		"theme.gen.css":                      "--primary-foreground:",
		"product_templ.go":                   "func ProductListPage(",
		"package.json":                       `"htmx.org"`,
		"messages.go":                        "var ownWords",
		"routes.go":                          "func (s *Site) ownRoutes(",
		"dist/README.md":                     "dist/assets",
		"../../package.json":                 `"web/shop"`,
		"../../cmd/shop/security_command.go": `"customer": {page: "/shop/nova-senha"},`,
	} {
		got := read(t, filepath.Join(site, path))
		if path == "product_gen.go" {
			want = "s.deps.Catalog.ReadProduct(ctx, id)"
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %q", path, want)
		}
	}
	if kit := read(t, filepath.Join(site, "kit.templ")); !strings.Contains(kit, `href={ templ.URL(Asset("site.css")) }`) || !strings.Contains(kit, `src={ Asset("kit.js") }`) {
		t.Error("the site's assets are not linked with their versions")
	}
	if gen := read(t, filepath.Join(site, "site_gen.go")); !strings.Contains(gen, `"public, max-age=31536000, immutable"`) {
		t.Error("an asset at its version is not kept for good")
	}
	routes := read(t, filepath.Join(dir, "cmd", "shop", "routes_gen.go"))
	if !strings.Contains(routes, "web.Handle(mux, shop.New(shop.Deps{") || !strings.Contains(routes, "Catalog: d.catalog,") {
		t.Errorf("the app does not serve the site:\n%s", routes)
	}
}

func TestSSRChecksItsPages(t *testing.T) {
	for name, tc := range map[string]struct {
		change, model func(string) string
		want          string
	}{
		"engine": {func(s string) string { return strings.Replace(s, "locale pt_br", "engine astro", 1) }, nil, "engine astro is not one esquadro generates yet"},
		"url":    {func(s string) string { return strings.Replace(s, "locale pt_br", "url \"shop.com\"", 1) }, nil, "url shop.com is not a site's address"},
		"path": {nil, func(s string) string { return strings.Replace(s, `path "produtos"`, `path "busca"`, 1) },
			"path busca of Product is where ssr shop has its search page"},
		"same": {nil, func(s string) string { return strings.Replace(s, `path "produtos"`, `path "categorias"`, 1) },
			"have the same path on shop: categorias"},
		"title": {nil, func(s string) string { return strings.Replace(s, "description description", "title nome", 1) },
			"title nome of @shop is not a field of Product"},
		"filter": {nil, func(s string) string {
			return strings.Replace(s, "category    > Category required @store(filter)", "category    > Category required", 1)
		},
			"which needs @store(filter)"},
		"readable": {nil, func(s string) string {
			return strings.Replace(s, "@security(read public, write [user.admin, user.staff])", "@security(read user, write [user.admin, user.staff])", 1)
		}, "Product has pages on shop, which nobody there may read"},
	} {
		_, code, errOut := ssrProject(t, tc.change, tc.model)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", name, code, tc.want, errOut)
		}
	}
}

func TestSSRSiteWithABrand(t *testing.T) {
	dir, code, errOut := ssrProject(t, nil, nil)
	if code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	os.WriteFile(appEsq, []byte(strings.Replace(read(t, appEsq), "    name \"Loja\"\n", "    name \"Loja\"\n    brand \"brand\"\n", 1)), 0o644)

	if code, _, errOut := esq(t, "check", dir); code == 0 || !strings.Contains(errOut, "brand brand:") {
		t.Fatalf("a brand without its folder checks: exit %d\n%s", code, errOut)
	}
	files := map[string]string{
		"fonts/Body.woff2":       "font",
		"logo.svg":               "<svg/>",
		"states/lost.svg":        "<svg/>",
		"icons/favicon.ico":      "ico",
		"icons/site.webmanifest": "{}",
		"theme.json": `{"colors": {"accent": "#2F6FEB", "on_accent": "#FFFFFF", "sale": "#E5484D"},
			"fonts": {"body": {"family": "Body Sans", "files": {"300 900": "fonts/Body.woff2"}}},
			"text": {"section": {"weight": 800, "size": 28}}, "buttons": "raised",
			"states": {"not_found": "states/lost.svg"}, "identity": {"logo": "logo.svg", "icons": "icons"}}`,
	}
	for name, content := range files {
		p := filepath.Join(dir, "brand", filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	site := filepath.Join(dir, "web", "shop")
	for path, want := range map[string][]string{
		"../../brand/embed.go": {"package brand", "//go:embed all:*"},
		"site_gen.go": {`brand "example.com/shop/brand"`, `m.Handle("GET /brand/"`, `"icons/favicon.ico"`,
			`"application/manifest+json"`, `"not_found": "states/lost.svg"`},
		"theme.gen.css": {`url("/shop/brand/fonts/Body.woff2")`, "--brand-accent: #2F6FEB;", "--color-sale: var(--brand-sale);", ".t-section {", "inset 0 -2px 0"},
		"kit.templ":     {`<link rel="manifest" href={ URL("/site.webmanifest") }/>`, `<meta name="theme-color" content="#2F6FEB"/>`, `URL("/brand/logo.svg")`, "templ StatePicture("},
	} {
		got := read(t, filepath.Join(site, path))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q", path, w)
			}
		}
	}
	vet := exec.Command("go", "vet", "./web/...", "./brand/...")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
}

// TestSSRDrawsItsIconsAndYours gives the site icons of the project's own in
// icons.go, and the kit draws them: one in place of Lucide's, a whole <svg>,
// none for an empty one.
func TestSSRDrawsItsIconsAndYours(t *testing.T) {
	dir, code, errOut := ssrProject(t, nil, nil)
	if code != 0 {
		t.Fatal(errOut)
	}
	site := filepath.Join(dir, "web", "shop")
	for path, want := range map[string]string{
		"icons_gen.go": `"share-2":`,
		"icons.go":     "var ownIcons = map[string]string{}",
		"kit.templ":    `@Icon("chevron-left")`,
	} {
		if got := read(t, filepath.Join(site, path)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q", path, want)
		}
	}
	icons := filepath.Join(site, "icons.go")
	own := "var ownIcons = map[string]string{\n\t\"share-2\":      `<path d=\"M1 1h2\"/>`,\n\t\"chevron-left\": \"\",\n\t\"mine\":         `<svg id=\"mine\"></svg>`,\n}"
	os.WriteFile(icons, []byte(strings.Replace(read(t, icons), "var ownIcons = map[string]string{}", own, 1)), 0o644)
	os.WriteFile(filepath.Join(site, "icons_own_test.go"), []byte(read(t, filepath.Join("testdata", "ssr", "icons_own_test.go.txt"))), 0o644)
	// And its words count: 1 encontrado, 2 encontrados.
	os.WriteFile(filepath.Join(site, "words_own_test.go"), []byte(read(t, filepath.Join("testdata", "ssr", "words_own_test.go.txt"))), 0o644)
	test := exec.Command("go", "test", "./web/shop/")
	test.Dir = dir
	if out, err := test.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
}

// withImages adds the storage and images blueprints to the site's app.
func withImages(s string) string {
	return strings.Replace(s, "  blueprint api_docs\n", "  blueprint api_docs\n  blueprint storage\n  blueprint images\n", 1)
}

func TestSSRChecksOGImage(t *testing.T) {
	ogProducts := func(s string) string {
		return strings.Replace(s, "sort \"-price\", menu)", "sort \"-price\", menu, og_image)", 1)
	}
	for name, tc := range map[string]struct {
		change, model func(string) string
		want          string
	}{
		"no images": {nil, ogProducts, "og_image of Product on shop draws each record's Open Graph image on the server, which needs blueprint images"},
		"no page": {withImages, func(s string) string {
			return strings.Replace(s, "  status      Status  default(ACTIVE)\n", "  status      Status  default(ACTIVE)\n  @shop(og_image)\n", 1)
		}, "og_image of Product is the image of its records' pages on shop, which it has not"},
	} {
		_, code, errOut := ssrProject(t, tc.change, tc.model)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", name, code, tc.want, errOut)
		}
	}
}

func TestSSRDrawsOGImages(t *testing.T) {
	dir, code, errOut := ssrProject(t, withImages, func(s string) string {
		s = strings.Replace(s, "sort \"-price\", menu)", "sort \"-price\", menu, og_image)", 1)
		return strings.Replace(s, "  status      Status  default(ACTIVE)\n", "  status      Status  default(ACTIVE)\n  photo       image\n", 1)
	})
	if code != 0 {
		t.Fatal(errOut)
	}
	site := filepath.Join(dir, "web", "shop")
	for path, want := range map[string][]string{
		"../../internal/platform/images/images.go":      {"func New(width, height int, theme Theme) *Canvas", "func (c *Canvas) Picture(", "OGWidth  = 1200"},
		"../../internal/platform/images/text.go":        {"func Wrap(", "func LoadFonts("},
		"../../internal/platform/images/cache.go":       {"func (c Cache) PNG(", `"example.com/shop/internal/platform/storage"`},
		"../../internal/platform/images/images_test.go": {"func TestWrapCutsWithAnEllipsis("},
		"../../internal/platform/storage/storage.go":    {"func (s *Store) Object(", "func (s *Store) DeleteObjects("},
		"site_gen.go": {`m.HandleFunc("GET /produtos/{id}/og.png", s.ogProduct)`, "const ogThemeHash = ", "func servePNG(",
			"func (s *Site) picture(", `"#2a85ff",`},
		"product_gen.go": {"type ProductOG struct", "Picture image.Image", "func (s *Site) ogProduct(", `"images/shop/products/"`,
			"view.ImageWidth, view.ImageHeight = images.OGWidth, images.OGHeight", "o.Description = summary(x.Description)"},
		"product_og.go": {"const productOGVersion = \"1\"", "func DrawProductOG(c *images.Canvas, o ProductOG) error", "c.Picture(card.Inset(28), o.Picture, images.Contain, 12)"},
		"og_test.go":    {"func TestOGProduct(t *testing.T) {", "m.CreateCategory(ctx, catalog.NewCategory{"},
		"kit.templ":     {`<meta property="og:image:width"`},
	} {
		got := read(t, filepath.Join(site, path))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q", path, w)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(site, "category_og.go")); err == nil {
		t.Error("Category has no og_image, yet has an image")
	}
	vet := exec.Command("go", "vet", "./web/...", "./internal/platform/images/...", "./internal/platform/storage/...")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
	test := exec.Command("go", "test", "./internal/platform/images/...")
	test.Dir = dir
	if out, err := test.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}

	// With a brand: its colors and fonts, and a warning of a font the
	// images cannot draw.
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	os.WriteFile(appEsq, []byte(strings.Replace(read(t, appEsq), "    name \"Loja\"\n", "    name \"Loja\"\n    brand \"brand\"\n", 1)), 0o644)
	for name, content := range map[string]string{
		"fonts/Body.woff2":  "font",
		"fonts/Display.ttf": "font",
		"logo.png":          "png",
		"theme.json": `{"colors": {"accent": "#E5484D", "page": "#1C1917"},
			"fonts": {"display": {"family": "Display", "files": {"700": "fonts/Display.ttf"}}, "body": {"family": "Body Sans", "files": {"300 900": "fonts/Body.woff2"}}},
			"identity": {"logo": "logo.png", "og_image": "logo.png"}}`,
	} {
		p := filepath.Join(dir, "brand", filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	code, _, errOut = esq(t, "gen", dir)
	if code != 0 || !strings.Contains(errOut, "the body font has no TrueType or OpenType file") {
		t.Fatalf("exit %d, want a warning of the body font:\n%s", code, errOut)
	}
	got := read(t, filepath.Join(site, "site_gen.go"))
	for _, w := range []string{`images.LoadFonts(brand.Files, map[string]string{"display": "fonts/Display.ttf"})`, `fs.ReadFile(brand.Files, "logo.png")`,
		`"#1C1917",`, `http.Redirect(w, r, URL("/brand/logo.png"), http.StatusFound)`} {
		if !strings.Contains(got, w) {
			t.Errorf("site_gen.go lacks %q", w)
		}
	}
	vet = exec.Command("go", "vet", "./web/...")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
}

// TestSSRDrawsOGImagesOnADomain runs the generated test of the Open Graph
// images of a site on a domain of its own, whose pages point at it.
func TestSSRDrawsOGImagesOnADomain(t *testing.T) {
	dir, code, errOut := ssrProject(t, func(s string) string {
		return withImages(strings.Replace(s, "    name \"Loja\"\n", "    name \"Loja\"\n    domain \"shop.test\"\n", 1))
	}, func(s string) string {
		s = strings.Replace(s, "sort \"-price\", menu)", "sort \"-price\", menu, og_image)", 1)
		return strings.Replace(s, "  status      Status  default(ACTIVE)\n", "  status      Status  default(ACTIVE)\n  photo       image\n", 1)
	})
	if code != 0 {
		t.Fatal(errOut)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "web", "shop", "site_gen.go")), `Domain = "shop.test"`) {
		t.Fatal("the site has no domain")
	}
	test := exec.Command("go", "test", "./web/shop/", "-run", "TestOG")
	test.Dir = dir
	if out, err := test.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
}

// seoProducts gives the shop's products a key, a price, a seller, and pages
// by category and by status, with the status they are in stock with.
func seoProducts(s string) string {
	s = strings.Replace(s, `schema product, sort "-price", menu)`, `schema product, key code, seller brand, menu)`, 1)
	return strings.Replace(s, "  price       decimal required min(0) @store(filter, sort)\n  status      Status  default(ACTIVE)\n  category    > Category required @store(filter)\n",
		"  price       money   required @store(sort)\n  code        string  unique max(12)\n  brand       string\n"+
			"  status      Status  default(ACTIVE) @store(filter) @shop(browse, path \"estado\", available [ACTIVE])\n"+
			"  category    > Category required @store(filter) @shop(browse)\n", 1)
}

// withCurrency gives the app the currency of its money, and the site a
// description.
func withCurrency(s string) string {
	s = strings.Replace(s, "    name \"Loja\"\n", "    name \"Loja\"\n    description \"Os produtos da loja.\"\n", 1)
	return strings.Replace(s, "  module \"example.com/shop\"\n", "  module \"example.com/shop\"\n  currency BRL\n", 1)
}

// TestSSRAddressesForSearchEngines runs, beside the site, a test of its
// addresses by key and their moves, its pages by category and by status,
// the structured data, the sitemap and llms.txt (seo_own_test.go).
func TestSSRAddressesForSearchEngines(t *testing.T) {
	dir, code, errOut := ssrProject(t, withCurrency, seoProducts)
	if code != 0 {
		t.Fatal(errOut)
	}
	site := filepath.Join(dir, "web", "shop")
	for path, want := range map[string][]string{
		"product_gen.go": {"func ProductsByCategoryPath(x catalog.Category) string", `return "/produtos/estado/" + Slug(StatusLabel(v))`,
			"x, err = s.deps.Catalog.ReadProductByCode(ctx, key)", `offer["availability"] = "https://schema.org/InStock"`},
		"site_gen.go":   {"func keyed(key string) bool", "type browse struct", `m.HandleFunc("GET /llms.txt", s.llms)`, `Description = "Os produtos da loja."`},
		"pages_gen.go":  {"func (s *Site) llms(", "s.productsBrowses(ctx)"},
		"kit.templ":     {"templ Breadcrumbs(crumbs []Crumb)", `<meta property="og:type" content={ p.Type }/>`},
		"product.templ": {"@Breadcrumbs(v.Crumbs)", "ProductsByStatusURL(v.Item.Status)"},
	} {
		got := read(t, filepath.Join(site, path))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q", path, w)
			}
		}
	}
	os.WriteFile(filepath.Join(site, "seo_own_test.go"), []byte(read(t, filepath.Join("testdata", "ssr", "seo_own_test.go.txt"))), 0o644)
	test := exec.Command("go", "test", "./web/shop/", "-run", "TestAddressesForSearchEngines", "-count", "1")
	test.Dir = dir
	if out, err := test.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}

	// shown.go, the project's, narrows what the site shows: to the active.
	if got := read(t, filepath.Join(site, "shown.go")); !strings.Contains(got, "func listedProducts(*catalog.ProductQuery) {}") || !strings.Contains(got, "func shownProduct(catalog.Product) bool { return true }") {
		t.Fatalf("shown.go:\n%s", got)
	}
	os.Remove(filepath.Join(site, "seo_own_test.go"))
	shown := read(t, filepath.Join(site, "shown.go"))
	shown = strings.Replace(shown, "func listedProducts(*catalog.ProductQuery) {}", "func listedProducts(q *catalog.ProductQuery) {\n\tq.Filter.Status = []catalog.Status{catalog.StatusActive}\n}", 1)
	shown = strings.Replace(shown, "func shownProduct(catalog.Product) bool { return true }", "func shownProduct(x catalog.Product) bool { return x.Status == catalog.StatusActive }", 1)
	os.WriteFile(filepath.Join(site, "shown.go"), []byte(shown), 0o644)
	os.WriteFile(filepath.Join(site, "shown_own_test.go"), []byte(read(t, filepath.Join("testdata", "ssr", "shown_own_test.go.txt"))), 0o644)
	test = exec.Command("go", "test", "./web/shop/", "-run", "TestShowsWhatTheProjectSays", "-count", "1")
	test.Dir = dir
	if out, err := test.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
}

func TestSSRChecksItsAddresses(t *testing.T) {
	for name, tc := range map[string]struct {
		model func(string) string
		want  string
	}{
		"key not unique": {func(s string) string {
			return strings.Replace(seoProducts(s), "code        string  unique max(12)", "code        string  max(12)", 1)
		}, "key of @shop: Product.code must be a unique string"},
		"key of none": {func(s string) string { return strings.Replace(seoProducts(s), "key code", "key sku", 1) }, "key sku of @shop is not a field of Product"},
		"browse no filter": {func(s string) string {
			return strings.Replace(seoProducts(s), "@store(filter) @shop(browse)", "@shop(browse)", 1)
		}, "browse of Product.category: its page lists products by it, which needs @store(filter)"},
		"browse text": {func(s string) string {
			return strings.Replace(seoProducts(s), "  brand       string\n", "  brand       string @shop(browse)\n", 1)
		}, "browse of Product.brand: a browse page is of an enum's value or a related record"},
		"path alone": {func(s string) string {
			return strings.Replace(seoProducts(s), "  brand       string\n", "  brand       string @shop(path \"marca\")\n", 1)
		}, `path of Product.brand is the segment of a browse page's address`},
		"bad path": {func(s string) string { return strings.Replace(seoProducts(s), `path "estado"`, `path "Estado X"`, 1) },
			`path "Estado X" of Product.status is no segment of an address`},
		"available": {func(s string) string {
			return strings.Replace(seoProducts(s), "available [ACTIVE]", "available [ON]", 1)
		},
			"available of Product.status: ON is not a value of Status"},
		"seller no product": {func(s string) string { return strings.Replace(seoProducts(s), "schema product, ", "", 1) },
			"seller of Product is of a product's structured data"},
	} {
		_, code, errOut := ssrProject(t, withCurrency, tc.model)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", name, code, tc.want, errOut)
		}
	}
}
