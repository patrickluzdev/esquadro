package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveEmptyParentsInTheCurrentDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join(".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	removeEmptyParents(".", filepath.Join(".github", "workflows"))
	if _, err := os.Stat(".github"); !os.IsNotExist(err) {
		t.Fatal("the empty folders stayed")
	}
}
