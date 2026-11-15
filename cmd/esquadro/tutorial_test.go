//go:build e2e

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTutorial follows docs/src/content/docs/tutorial.md step by step and
// checks what the page says each step generates, so the tutorial keeps
// telling the truth. A change here usually means a change to the page.
func TestTutorial(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(cache, "esquadro-tutorial-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	dir := filepath.Join(root, "shop")
	e := &lifecycle{t: t, dir: dir}
	model := func(name string) string { return filepath.Join(dir, "esquadro", name) }

	// 1. Start.
	e.esquadro(0, "new", "-starter", "api", "-module", "example.com/shop", dir)
	db := e.postgres()
	e.run(db, "go", "run", "./cmd/shop", "migrate")
	// step regenerates, migrates and serves, as the tutorial does after each
	// change; it returns how to stop the service.
	step := func() func() {
		t.Helper()
		e.esquadro(0, "gen", dir)
		e.run(db, "go", "run", "./cmd/shop", "migrate")
		return e.serve(db)
	}
	stop := e.serve(db)
	category := e.call("POST", "/categories", `{"name":"Books"}`, 201)
	stop()

	// 2. Add a field: a migration adding the column, and a taken sku refused.
	e.write(model("catalog.esq"), addFields(t, e.read(model("catalog.esq")), "Product", "  sku string unique"))
	stop = step()
	name := only(t, filepath.Join(dir, "migrations", "00002_*.sql"))
	if filepath.Base(name) != "00002_add_products_sku.sql" {
		t.Fatalf("the tutorial names the migration 00002_add_products_sku.sql, esquadro wrote %s", filepath.Base(name))
	}
	migration := e.read(name)
	for _, want := range []string{"ALTER TABLE public.products ADD COLUMN sku text;", "ALTER TABLE public.products ADD CONSTRAINT products_sku_key UNIQUE (sku);"} {
		if !strings.Contains(migration, want) {
			t.Fatalf("the migration does not say %q:\n%s", want, migration)
		}
	}
	newProduct := func(name, sku string) string {
		return fmt.Sprintf(`{"name":%q,"price":"10","categoryId":%q,"sku":%q}`, name, category["id"], sku)
	}
	product := e.call("POST", "/products", newProduct("Dune", "DU-1"), 201)
	refused := e.call("POST", "/products", newProduct("Copy", "DU-1"), 422)
	if !strings.Contains(fmt.Sprint(refused["errors"]), "code:taken field:sku") {
		t.Fatalf("a taken sku: %v", refused)
	}
	stop()

	// 3. Add an entity.
	catalog := addFields(t, e.read(model("catalog.esq")), "Product", "  brand > Brand")
	e.write(model("catalog.esq"), catalog+"\n/// Who makes a product.\nentity Brand {\n  name string required unique\n}\n")
	stop = step()
	e.call("POST", "/brands", `{"name":"Ace"}`, 201)
	stop()

	// 4. Tag products.
	catalog = addFields(t, e.read(model("catalog.esq")), "Product", "  tags <> Tag")
	e.write(model("catalog.esq"), catalog+"\nentity Tag {\n  label string required unique\n}\n")
	stop = step()
	tag := e.call("POST", "/tags", `{"label":"sci-fi"}`, 201)
	link := fmt.Sprintf("/products/%s/tags/%s", product["id"], tag["id"])
	e.call("PUT", link, "", 204)
	if page := e.call("GET", fmt.Sprintf("/products/%s/tags", product["id"]), "", 200); len(page["items"].([]any)) != 1 {
		t.Fatalf("tags of the product: %v", page)
	}
	if page := e.call("GET", fmt.Sprintf("/tags/%s/products", tag["id"]), "", 200); len(page["items"].([]any)) != 1 {
		t.Fatalf("products of the tag: %v", page)
	}
	e.call("DELETE", link, "", 204)
	stop()

	// 5. A second module, in a schema of its own.
	e.write(model("orders.esq"), `package orders

/// A purchase.
entity Order {
  number    string required unique
  placed_at time   default(now)
}

/// One product in an order.
entity OrderItem {
  quantity int required min(1)
  order    > Order required on_delete(cascade)
  product  > Product required
}
`)
	app := e.read(model("app.esq"))
	e.write(model("app.esq"), strings.Replace(app, "  blueprint postgres\n", "  blueprint postgres {\n    schemas [orders]\n  }\n", 1))
	stop = step()
	order := e.call("POST", "/orders", `{"number":"A-1"}`, 201)
	e.call("POST", "/order-items", fmt.Sprintf(`{"quantity":2,"orderId":%q,"productId":%q}`, order["id"], product["id"]), 201)
	if m := e.read(only(t, filepath.Join(dir, "migrations", "00005_*.sql"))); !strings.Contains(m, "CREATE SCHEMA orders;") {
		t.Fatalf("the orders' schema was not created:\n%s", m)
	}
	stop()

	// 6. Answer in Portuguese.
	app = e.read(model("app.esq"))
	e.write(model("app.esq"), strings.Replace(app, "  blueprint i18n\n", "  blueprint i18n {\n    languages [\"en\", \"pt-BR\"]\n  }\n", 1))
	stop = step()
	req, _ := http.NewRequest("GET", e.base+"/api/v1/nowhere", nil)
	req.Header.Set("Accept-Language", "pt-BR")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 404 || !strings.Contains(string(body), "Nada foi encontrado neste endereço.") {
		t.Fatalf("in Portuguese: %d %s", res.StatusCode, body)
	}
	stop()

	// 7. Your own code is left alone.
	mine := filepath.Join(dir, "internal", "catalog", "discount.go")
	e.write(mine, "package catalog\n\n// Discounted is written by hand.\nfunc Discounted(p Product) bool { return false }\n")
	e.esquadro(0, "gen", dir)
	if !strings.Contains(e.read(mine), "written by hand") {
		t.Fatal("gen changed a file of the user's")
	}
	e.run(nil, "go", "vet", "./...")
}

// addFields adds lines at the end of an entity of an .esq source.
func addFields(t *testing.T, src, entity string, lines ...string) string {
	t.Helper()
	start := strings.Index(src, "entity "+entity+" {")
	if start < 0 {
		t.Fatalf("no entity %s in:\n%s", entity, src)
	}
	end := start + strings.Index(src[start:], "\n}")
	return src[:end] + "\n" + strings.Join(lines, "\n") + src[end:]
}

// only is the single file a glob matches.
func only(t *testing.T, pattern string) string {
	t.Helper()
	matches, _ := filepath.Glob(pattern)
	if len(matches) != 1 {
		t.Fatalf("%s matches %v, want one file", pattern, matches)
	}
	return matches[0]
}
