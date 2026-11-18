package seed

import (
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestOrderedPutsTargetsFirst(t *testing.T) {
	category, product := &project.Entity{}, &project.Entity{}
	category.Raw, product.Raw = "Category", "Product"
	product.Relations = []*project.Relation{{Kind: project.ManyToOne, Entity: product, Target: category}}
	got := Ordered([]*project.Entity{product, category})
	if got[0] != category || got[1] != product {
		t.Fatalf("order %s, %s", got[0].Raw, got[1].Raw)
	}
}
