package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every locale has every word: a frontend in any of them shows them all.
func TestWordsInEveryLocale(t *testing.T) {
	en := Words("en")
	if len(en) == 0 {
		t.Fatal("no words in en")
	}
	for _, locale := range Locales {
		got := Words(locale)
		if len(got) != len(en) {
			t.Fatalf("%s has %d words, en %d", locale, len(got), len(en))
		}
		for i, w := range got {
			if w.Key != en[i].Key || w.Text == "" {
				t.Fatalf("%s: word %d is %q (%q), en's %q", locale, i, w.Key, w.Text, en[i].Key)
			}
		}
	}
}

// TestKitsSayNothingOutsideTheWords keeps the frontends' templates from
// writing words of their own that a screen or a screen reader shows (an
// aria-label, a hidden label, a title, a placeholder, a button's text):
// they come from the words, in the frontend's language.
func TestKitsSayNothingOutsideTheWords(t *testing.T) {
	fixed := regexp.MustCompile(`(aria-label|title|placeholder|alt)="[A-Za-z][^"{]*"|className="[^"]*sr-only[^"]*">[A-Za-z]|<Button[^>]*>[A-Z][a-z]+<|<(SheetTitle|SheetDescription|DialogTitle|DialogDescription)>[A-Za-z]`)
	for _, root := range []string{"../spa/templates", "../ssr/templates"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".tmpl") || strings.Contains(path, "test") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
					continue
				}
				if m := fixed.FindString(line); m != "" {
					t.Errorf("%s:%d says %s outside the words", path, i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
