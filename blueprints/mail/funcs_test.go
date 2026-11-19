package mail

import (
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
)

// Every locale a frontend has writes every word of the layout, each with
// the app's name.
func TestWordsInEveryLocale(t *testing.T) {
	en := Words("en")
	if len(en) == 0 {
		t.Fatal("no words in en")
	}
	for _, locale := range ui.Locales {
		got := Words(locale)
		if len(got) != len(en) {
			t.Fatalf("%s has %d words, en %d", locale, len(got), len(en))
		}
		for i, w := range got {
			if w.Key != en[i].Key || !strings.Contains(w.Text, "{app}") {
				t.Errorf("%s: word %d is %q (%q), en's %q", locale, i, w.Key, w.Text, en[i].Key)
			}
		}
	}
	if got := Words("fr"); len(got) != len(en) || got[0].Text != en[0].Text {
		t.Errorf("a locale esquadro does not know has English: %v", got)
	}
}

func TestBrandDir(t *testing.T) {
	for opt, want := range map[string]string{"": "brand", "/": "brand", "marca/": "marca", "assets/brand": "assets/brand"} {
		if got := BrandDir(map[string]any{"brand": opt}); got != want {
			t.Errorf("BrandDir(%q) = %s, want %s", opt, got, want)
		}
	}
}
