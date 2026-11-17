package project

import (
	"strings"
	"testing"
)

func TestReports(t *testing.T) {
	src := `package analytics

app shop {
  module "example.com/shop"
}

/// Clicks per niche.
report ClicksByNiche {
  niche     string  required
  clicks    int     required
  per_offer decimal
  since     time    required param
  network   string  param
}
`
	p, diags := Load(Source{Name: "analytics.esq", Src: []byte(src)})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if len(p.Reports) != 1 || len(p.Modules) != 1 || len(p.Modules[0].Reports) != 1 || len(p.Entities) != 0 {
		t.Fatalf("reports %d, modules %d", len(p.Reports), len(p.Modules))
	}
	r := p.Reports[0]
	if r.Pascal != "ClicksByNiche" || r.Snake != "clicks_by_niche" || r.Module.Raw != "analytics" || r.Doc[0] != "Clicks per niche." {
		t.Fatalf("report: %+v", r)
	}
	if len(r.Columns) != 3 || len(r.Params) != 2 || !r.Columns[0].Required || r.Columns[2].Required || !r.Params[0].Required || r.Params[1].Required {
		t.Fatalf("columns %+v, params %+v", r.Columns, r.Params)
	}
	if d := p.Describe(); len(d.Reports) != 1 || len(d.Reports[0].Columns) != 3 || len(d.Reports[0].Params) != 2 {
		t.Fatalf("described: %+v", d.Reports)
	}
}

func TestReportErrors(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"relation": {"  niche > Niche\n  clicks int", "cannot be relations"},
		"unique":   {"  clicks int unique", "unique is for entities"},
		"default":  {"  clicks int default(0)", "default is for entities"},
		"json":     {"  extra json", "json is not one"},
		"empty":    {"  since time param", "has no columns"},
		"twice":    {"  clicks int\n  clicks int param", "already a column or parameter"},
		"param":    {"  clicks int param(1)", "param takes no arguments"},
	} {
		src := "package analytics\n\napp shop {\n  module \"example.com/shop\"\n}\n\nentity Niche {\n  name string\n}\n\nreport R {\n" + tc.body + "\n}\n"
		_, diags := Load(Source{Name: "a.esq", Src: []byte(src)})
		if !diags.HasErrors() || !strings.Contains(diags.Err().Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, diags, tc.want)
		}
	}
}
