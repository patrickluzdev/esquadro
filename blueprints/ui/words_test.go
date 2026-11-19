package ui

import "testing"

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
