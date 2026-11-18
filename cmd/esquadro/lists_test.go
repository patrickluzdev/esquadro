package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListsCheckTheModel(t *testing.T) {
	dir, appEsq := smallProject(t, "app,env,domain,http,postgres,migrations,store,rest")
	src := strings.Replace(read(t, appEsq), "  blueprint store\n", "  blueprint store {\n    search_language klingon\n  }\n", 1)
	src = strings.Replace(src, "  blueprint rest\n", "  blueprint rest {\n    pagination [cursor, offset]\n  }\n", 1)
	os.WriteFile(appEsq, []byte(src), 0o644)
	os.WriteFile(filepath.Join(dir, "esquadro", "catalog.esq"), []byte(`package catalog

entity Product {
  name  string required @store(search, sort)
  price int    @store(sort)
  size  int    @store(search)
  data  json   @store(filter)
  @rest(pagination [pages], unpaged_max 0)
}
`), 0o644)
	code, _, errOut := esq(t, "check", dir)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	for _, want := range []string{
		`sort: Product.price may be empty`,
		`search: Product.size is int`,
		`filter: Product.data is json`,
		`search_language "klingon" is not one of PostgreSQL's`,
		`pagination "offset" is not cursor or page`,
		`Product: pagination "pages" is not cursor or page`,
		`Product: unpaged_max 0 must be at least 1`,
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("missing %q in:\n%s", want, errOut)
		}
	}
}

func TestListsRefuseFiltersNamedLikeTheirParameters(t *testing.T) {
	dir, _ := smallProject(t, "app,env,domain,http,postgres,migrations,store,rest")
	os.WriteFile(filepath.Join(dir, "esquadro", "catalog.esq"), []byte(`package catalog

entity Visit {
  page      string required @store(filter)
  sort      string required
  price     int    required @store(filter)
  price_min int    required @store(filter)
}
`), 0o644)
	code, _, errOut := esq(t, "check", dir)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	for _, want := range []string{
		`catalog.esq:4:3: error: Visit: a filter is the list's parameter page`,
		`catalog.esq:7:3: error: Visit: two filters are the parameter price_min`,
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("missing %q in:\n%s", want, errOut)
		}
	}
	// A field named like a parameter but not filtered is fine.
	if strings.Contains(errOut, "parameter sort") {
		t.Errorf("a field not filtered was refused:\n%s", errOut)
	}
}
