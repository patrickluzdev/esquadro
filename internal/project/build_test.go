package project

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

var update = flag.Bool("update", false, "rewrite golden files")

func load(t *testing.T, path string) (*Project, diag.List, []byte) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, diags := Load(Source{Name: filepath.Base(path), Src: src})
	return p, diags, src
}

// TestErrorsGolden builds every testdata/errors/*.esq file and compares the
// rendered diagnostics to the .golden file next to it.
func TestErrorsGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/errors/*.esq")
	if len(files) == 0 {
		t.Fatal("no test files")
	}
	for _, path := range files {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".esq"), func(t *testing.T) {
			_, diags, src := load(t, path)
			if !diags.HasErrors() {
				t.Fatal("expected errors, got none")
			}
			var b strings.Builder
			diag.Render(&b, diags, func(string) []byte { return src })
			got := b.String()
			golden := strings.TrimSuffix(path, ".esq") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run go test ./internal/project -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("output differs from %s (run with -update to accept)\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
			}
		})
	}
}

func TestShopModel(t *testing.T) {
	p, diags, _ := load(t, "testdata/ok/shop.esq")
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if p.App.Raw != "shop" || p.App.Module != "github.com/example/shop" {
		t.Errorf("app = %q %q", p.App.Raw, p.App.Module)
	}
	if len(p.App.Blueprints) != 2 || p.App.Blueprints[1].Ref != "postgres" ||
		p.App.Blueprints[1].Source != SourceBuiltin || len(p.App.Blueprints[1].Options) != 1 {
		t.Errorf("blueprints = %+v", p.App.Blueprints)
	}

	status := p.Enum("ProductStatus")
	if status == nil || len(status.Values) != 3 || status.Values[2].Snake != "sold_out" {
		t.Fatalf("enum ProductStatus = %+v", status)
	}

	prod := p.Entity("Product")
	if prod == nil {
		t.Fatal("no Product")
	}
	if prod.Plural.Snake != "products" || prod.Plural.Pascal != "Products" {
		t.Errorf("plural = %+v", prod.Plural)
	}
	if prod.Doc[0] != "Something we sell." {
		t.Errorf("doc = %q", prod.Doc)
	}

	var fieldNames []string
	for _, f := range prod.Fields {
		fieldNames = append(fieldNames, f.Snake)
	}
	if got := strings.Join(fieldNames, " "); got != "id name description price stock status created_at updated_at" {
		t.Errorf("fields = %s", got)
	}
	if id := prod.Fields[0]; !id.Implicit || id.Type != TypeUUID || !id.Required {
		t.Errorf("id = %+v", id)
	}

	name := prod.Field("name")
	if !name.Required || name.Min.Value != 3 || name.Max.Text != "120" || name.Type != TypeString {
		t.Errorf("name = %+v", name)
	}
	st := prod.Field("status")
	if st.Type != TypeEnum || st.Enum != status || st.Default.Kind != DefaultEnum || st.Default.EnumValue != status.Value("ACTIVE") {
		t.Errorf("status = %+v", st)
	}
	if d := prod.Field("stock").Default; d.Kind != DefaultLiteral || d.Text != "0" {
		t.Errorf("stock default = %+v", d)
	}

	cat := p.Entity("Category")
	tag := p.Entity("Tag")
	rels := map[string]*Relation{}
	for _, r := range prod.Relations {
		rels[r.Snake] = r
	}
	c := rels["category"]
	if c == nil || c.Kind != ManyToOne || c.Target != cat || !c.Required || c.Display != cat.Field("name") || !c.OwnsForeignKey() {
		t.Fatalf("category = %+v", c)
	}
	inv := c.Inverse
	if inv == nil || inv.Entity != cat || inv.Snake != "products" || inv.Kind != OneToMany || !inv.Inferred || inv.Inverse != c || inv.OwnsForeignKey() {
		t.Fatalf("inverse of category = %+v", inv)
	}
	if len(cat.Relations) != 1 || cat.Relations[0] != inv {
		t.Errorf("Category relations = %+v", cat.Relations)
	}

	tags := rels["tags"]
	if tags == nil || tags.Kind != ManyToMany || tags.Target != tag || tags.JoinTable != "product_tags" {
		t.Fatalf("tags = %+v", tags)
	}
	if ti := tags.Inverse; ti.Entity != tag || ti.Snake != "products" || ti.Kind != ManyToMany || ti.JoinTable != "product_tags" {
		t.Errorf("inverse of tags = %+v", ti)
	}
}

func TestInverseNaming(t *testing.T) {
	src := `app a { module "x.com/a"
  blueprint platform
}
entity Person (plural "people") {
  name    string
  manager -> Person
  mentor  -> Person inverse(mentees)
  badge   -> Badge unique
}
entity Badge {
  code string
}
entity Order {
  buyer  -> Person
  seller -> Person
}
`
	p, diags := Load(Source{Name: "t.esq", Src: []byte(src)})
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	got := map[string]string{}
	for _, e := range p.Entities {
		for _, r := range e.Relations {
			if r.Inferred {
				got[e.Raw+"."+r.Snake] = r.Kind.String()
			}
		}
	}
	want := map[string]string{
		"Person.people_by_manager": "one-to-many", // self relation
		"Person.mentees":           "one-to-many", // explicit inverse(...)
		"Badge.person":             "one-to-one",  // singular for one-to-one
		"Person.orders_by_buyer":   "one-to-many", // two relations Order -> Person
		"Person.orders_by_seller":  "one-to-many",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d inverses, want %d: %v", len(got), len(want), got)
	}
	if d := p.Entity("Badge").Relations[0].Display; d.Raw != "name" {
		t.Errorf("default display of Person = %s, want name", d.Raw)
	}
	if d := p.Entity("Person").Field("name"); d == nil {
		t.Fatal("no name field")
	}
	if r := p.Entity("Order").Relations[0]; r.Display.Raw != "name" {
		t.Errorf("Order.buyer display = %s", r.Display.Raw)
	}
	if r := p.Entity("Person").Relations; r[2].Display.Raw != "code" {
		t.Errorf("Person.badge display = %s, want code (first string field)", r[2].Display.Raw)
	}
}

func TestSyntaxErrorsSkipSemantics(t *testing.T) {
	p, diags := Load(Source{Name: "t.esq", Src: []byte("entity A {\n  x\n}\n")})
	if p != nil || len(diags) != 1 {
		t.Fatalf("want only the syntax error, got %v", diags)
	}
}

func loadDir(t *testing.T, dir string) (*Project, diag.List, map[string][]byte) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*.esq"))
	srcs := map[string][]byte{}
	var sources []Source
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		srcs[filepath.Base(p)] = src
		sources = append(sources, Source{Name: filepath.Base(p), Src: src})
	}
	p, diags := Load(sources...)
	return p, diags, srcs
}

func TestMultiFileProject(t *testing.T) {
	p, diags, _ := loadDir(t, "testdata/multi/ok")
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := strings.Join(p.Files, " "); got != "app.esq catalog.esq orders.esq" {
		t.Errorf("files = %s", got)
	}
	order := p.Entity("Order")
	if order == nil || order.Field("status").Enum != p.Enum("OrderStatus") {
		t.Fatal("enum declared after its use in the same file was not resolved")
	}
	items := order.Relations[0]
	if items.Target != p.Entity("Product") || items.Inverse.Entity != p.Entity("Product") {
		t.Fatalf("relation across files not resolved: %+v", items)
	}
	if p.Entity("Product").Pos.File != "catalog.esq" {
		t.Errorf("Product position = %v", p.Entity("Product").Pos)
	}
}

func TestMultiFileErrors(t *testing.T) {
	_, diags, srcs := loadDir(t, "testdata/multi/errors")
	var b strings.Builder
	diag.Render(&b, diags, func(name string) []byte { return srcs[name] })
	got := b.String()
	golden := "testdata/multi/errors.golden"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden file (run go test ./internal/project -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}

func TestDescribeIsPlainJSON(t *testing.T) {
	p, diags, _ := load(t, "testdata/ok/shop.esq")
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	data, err := json.MarshalIndent(p.Describe(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`"module": "github.com/example/shop"`,
		`"snake": "sold_out"`,
		`"kind": "enum"`,
		`"target": "Category"`,
		`"inverse": "products"`,
		`"join_table": "product_tags"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("description missing %s", want)
		}
	}
	var back Description
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Entities[1].Fields[5].Default.Kind != DefaultEnum {
		t.Errorf("default kind did not round-trip: %+v", back.Entities[1].Fields[5])
	}
}
