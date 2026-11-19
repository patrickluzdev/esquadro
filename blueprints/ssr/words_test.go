package ssr

import (
	"testing"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
)

// Every locale has every word, and none of the site's is one of ui's.
func TestWordsInEveryLocale(t *testing.T) {
	en := Words("en")
	seen := map[string]bool{}
	for _, w := range en {
		if seen[w.Key] {
			t.Fatalf("%s is both ui's and the site's", w.Key)
		}
		seen[w.Key] = true
	}
	for _, locale := range ui.Locales {
		got := Words(locale)
		if len(got) != len(en) {
			t.Fatalf("%s has %d words, en %d", locale, len(got), len(en))
		}
		for i, w := range got {
			if w.Key != en[i].Key || w.Text == "" {
				t.Fatalf("%s: word %q, en's %q", locale, w.Key, en[i].Key)
			}
		}
	}
}
