package spa

import (
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestKitDrawsItsIconsFromTheSet reads the panel's files: each draws its
// icons from src/esquadro/icons.gen.ts, never from lucide-react, and every
// icon one imports is one of the kit's.
func TestKitDrawsItsIconsFromTheSet(t *testing.T) {
	importOf := regexp.MustCompile(`import\s*(type\s*)?\{([^}]*)\}\s*from\s*"([^"]+)"`)
	web := os.DirFS("templates/web/{{.Instance.Name}}")
	seen := 0
	err := fs.WalkDir(web, "src", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(path, "icons.") {
			return err
		}
		data, err := fs.ReadFile(web, path)
		if err != nil {
			return err
		}
		// The account menu's links are the project's to add to, next to
		// its icons' import: it keeps lucide-react, so that an upgrade
		// merges into a project's links without a conflict.
		if strings.Contains(string(data), `from "lucide-react"`) && !strings.HasSuffix(path, "user-menu.tsx.once.tmpl") {
			t.Errorf("%s imports lucide-react, not the icon set", path)
		}
		for _, m := range importOf.FindAllStringSubmatch(string(data), -1) {
			if m[1] != "" || !strings.HasSuffix(m[3], "icons.gen.ts") {
				continue
			}
			seen++
			for _, item := range strings.Split(m[2], ",") {
				name, _, _ := strings.Cut(strings.TrimSpace(item), " as ")
				if name == "" || strings.HasPrefix(name, "type ") || name == "icons" || name == "iconOf" {
					continue
				}
				if !slices.Contains(KitIcons, name) {
					t.Errorf("%s draws %s, which is not in KitIcons", path, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 40 {
		t.Errorf("only %d files import the icon set", seen)
	}
}

func TestIconSetAddsTheModelsIcons(t *testing.T) {
	got := IconSet(WithIcon(WithIcon(WithIcon(nil, "shopping_cart"), nil), "trash-2"))
	if !slices.IsSorted(got) || len(got) != len(KitIcons)+1 {
		t.Errorf("not the kit's icons and one more, sorted, each once: %v", got)
	}
	for _, want := range []string{"ShoppingCart", "Circle", "Trash2", "ChartColumn"} {
		if !slices.Contains(got, want) {
			t.Errorf("lacks %s", want)
		}
	}
}
