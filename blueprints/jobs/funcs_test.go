package jobs

import (
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestRefusesEmptyAndCanSample(t *testing.T) {
	src := "package a\napp x {\n  module \"x\"\n}\njob A {\n  id uuid required\n}\njob B {\n  n int required\n  s string default(\"x\") required\n}\njob C {\n  code string required pattern(\"^[A-Z]+$\")\n}\n"
	p, diags := project.Load(project.Source{Name: "a.esq", Src: []byte(src)})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	a, b, c := p.Jobs[0], p.Jobs[1], p.Jobs[2]
	if !refusesEmpty(a) || refusesEmpty(b) || !refusesEmpty(c) {
		t.Fatal("refusesEmpty")
	}
	if !canSample(a) || !canSample(b) || canSample(c) {
		t.Fatal("canSample")
	}
}
