package migrations

import (
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/project"
)

var update = flag.Bool("update", false, "rewrite golden files")

// load reads a version of a test project, resolving its blueprints so the
// @postgres annotations are filled in.
func load(t *testing.T, dir string) project.Description {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*.esq"))
	var sources []project.Source
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, project.Source{Name: filepath.Base(p), Src: src})
	}
	p, diags := project.Load(sources...)
	if diags.HasErrors() {
		t.Fatalf("%s: %v", dir, diags)
	}
	l := &blueprint.Loader{Builtin: blueprints.FS, HasBuiltinHook: func(string, string) bool { return true }}
	if _, diags := blueprint.Resolve(p, l); diags.HasErrors() {
		t.Fatalf("%s: %v", dir, diags)
	}
	return p.Describe()
}

// snapshot round-trips a description through JSON, as the saved snapshot of
// the previous generation does.
func snapshot(t *testing.T, d project.Description) *project.Description {
	t.Helper()
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var back project.Description
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	return &back
}

// TestMigrationsGolden writes the migrations between successive versions of
// testdata/shop (v1, v2, ...) and compares them to testdata/shop.golden.
func TestMigrationsGolden(t *testing.T) {
	versions, _ := filepath.Glob("testdata/shop/v*")
	var prev *project.Description
	var b strings.Builder
	for _, v := range versions {
		cur := load(t, v)
		m := Diff(prev, cur)
		fmt.Fprintf(&b, "======== %s: %s\n", filepath.Base(v), m.Name)
		for _, w := range m.Warnings {
			fmt.Fprintf(&b, "warning: %s\n", w)
		}
		b.WriteString(m.File())
		prev = snapshot(t, cur)
	}
	got := b.String()
	golden := "testdata/shop.golden"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden file (run go test ./blueprints/postgres -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s\n--- got ---\n%s", golden, got)
	}
}

func TestNoChangeGivesAnEmptyMigration(t *testing.T) {
	cur := load(t, "testdata/shop/v1")
	m := Diff(snapshot(t, cur), cur)
	if m.Up != "" || m.Down != "" || m.Name != "no_change" {
		t.Fatalf("migration = %+v", m)
	}
}

func TestFileNameNumbersAndReuses(t *testing.T) {
	dir := t.TempDir()
	cur := load(t, "testdata/shop/v1")
	first := Diff(nil, cur)

	name, err := FileName(dir, first)
	if err != nil || name != "migrations/00001_init.sql" {
		t.Fatalf("first = %q, %v", name, err)
	}
	os.MkdirAll(filepath.Join(dir, "migrations"), 0o755)
	os.WriteFile(filepath.Join(dir, name), []byte(first.File()), 0o644)

	// The same migration again (a generation that failed after writing it)
	// reuses the file.
	if again, _ := FileName(dir, first); again != name {
		t.Fatalf("again = %q", again)
	}
	next := Diff(snapshot(t, cur), load(t, "testdata/shop/v2"))
	if n, _ := FileName(dir, next); n != "migrations/00002_add_catalog_status_sold_out_and_more.sql" {
		t.Fatalf("next = %q", n)
	}
	if n, _ := FileName(dir, Diff(snapshot(t, cur), cur)); n != "" {
		t.Fatalf("no change must give no file, got %q", n)
	}
}

// withSchemas returns a copy of d whose postgres blueprint lists schemas.
func withSchemas(t *testing.T, d project.Description, schemas ...string) project.Description {
	t.Helper()
	c := *snapshot(t, d)
	list := make([]any, len(schemas))
	for i, s := range schemas {
		list[i] = s
	}
	for i, b := range c.App.Blueprints {
		if b.Name == "postgres" {
			opts := maps.Clone(b.Options)
			if opts == nil {
				opts = map[string]any{}
			}
			opts["schemas"] = list
			c.App.Blueprints[i].Options = opts
		}
	}
	return c
}

func TestMovingAModuleToItsOwnSchemaIsATodo(t *testing.T) {
	cur := load(t, "testdata/shop/v3")
	moved := withSchemas(t, cur, "catalog")
	m := Diff(snapshot(t, cur), moved)
	if strings.Contains(m.Up, "DROP TABLE") || strings.Contains(m.Up, "CREATE TABLE") {
		t.Fatalf("moving tables must never drop and recreate them:\n%s", m.Up)
	}
	for _, want := range []string{
		"CREATE SCHEMA catalog;",
		"-- TODO: table public.products is now catalog.products: move it by hand (ALTER TABLE public.products SET SCHEMA catalog",
		"-- TODO: enum type public.catalog_status is now catalog.status",
	} {
		if !strings.Contains(m.Up, want) {
			t.Errorf("up misses %q:\n%s", want, m.Up)
		}
	}
}

func TestCheckFindsTablesThatCollideInPublic(t *testing.T) {
	d := load(t, "testdata/shop/v1")
	if p := Check(d); len(p) != 0 {
		t.Fatalf("problems = %v", p)
	}
	for i, e := range d.Entities {
		if e.Raw == "Order" {
			d.Entities[i].Plural.Snake = "products" // as @postgres(table products) would
		}
	}
	d = withSchemas(t, d)
	problems := Check(withSchemas(t, d, "nope"))
	if len(problems) != 2 || !strings.Contains(problems[0], `"nope", which is not a module`) ||
		!strings.Contains(problems[1], "catalog.Product and orders.Order would both be table public.products") {
		t.Fatalf("problems = %v", problems)
	}
}

// softDeleting is d with @store(soft_delete) on the entity named.
func softDeleting(t *testing.T, d project.Description, entity string) project.Description {
	t.Helper()
	out := *snapshot(t, d)
	for i, e := range out.Entities {
		if e.Raw == entity {
			if out.Entities[i].Ann == nil {
				out.Entities[i].Ann = map[string]map[string]any{}
			}
			out.Entities[i].Ann["store"] = map[string]any{"soft_delete": true}
		}
	}
	return out
}

func TestSoftDeleteGivesATrashAndUniquenessAmongTheLive(t *testing.T) {
	cur := load(t, "testdata/shop/v1")
	trashed := softDeleting(t, cur, "Category")
	on := Diff(snapshot(t, cur), trashed)
	for _, want := range []string{
		"ALTER TABLE public.categories DROP CONSTRAINT categories_name_key;",
		"ALTER TABLE public.categories ADD COLUMN deleted_at timestamptz;",
		"CREATE UNIQUE INDEX categories_name_key ON public.categories (name) WHERE deleted_at IS NULL;",
		"CREATE INDEX categories_created_at_id_idx ON public.categories (created_at, id) WHERE deleted_at IS NULL;",
	} {
		if !strings.Contains(on.Up, want) {
			t.Errorf("turning it on misses %q:\n%s", want, on.Up)
		}
	}
	if strings.Contains(on.Up, "products") {
		t.Errorf("only the entity marked changes:\n%s", on.Up)
	}
	off := Diff(snapshot(t, trashed), cur)
	for _, want := range []string{
		"DROP INDEX public.categories_name_key;",
		"ALTER TABLE public.categories DROP COLUMN deleted_at;",
		"ALTER TABLE public.categories ADD CONSTRAINT categories_name_key UNIQUE (name);",
	} {
		if !strings.Contains(off.Up, want) {
			t.Errorf("turning it off misses %q:\n%s", want, off.Up)
		}
	}
	if again := Diff(snapshot(t, trashed), trashed); strings.TrimSpace(again.Up) != "" {
		t.Errorf("no change, no migration:\n%s", again.Up)
	}
}
