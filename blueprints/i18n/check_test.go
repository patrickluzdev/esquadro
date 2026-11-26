package i18n

import "testing"

func TestTags(t *testing.T) {
	for _, tc := range []struct {
		lang, tag string
		ok        bool
	}{
		{"en", "en", true},
		{"pt-BR", "pt-BR", true},
		{"es-419", "es-419", true},
		{"pt_br", "pt-BR", false},
		{"PT-br", "pt-BR", false},
		{"english", "english", false},
	} {
		if got := tagPattern.MatchString(tc.lang); got != tc.ok {
			t.Errorf("%s is a tag: %v, want %v", tc.lang, got, tc.ok)
		}
		if got := TagOf(tc.lang); got != tc.tag {
			t.Errorf("TagOf(%s) = %s, want %s", tc.lang, got, tc.tag)
		}
	}
}

// Each problem has its title in every language esquadro knows.
func TestEveryProblemTranslated(t *testing.T) {
	for lang := range translations {
		for _, p := range Problems {
			if _, ok := translations[lang][p.Code]; !ok {
				t.Errorf("%s has no title in %s", p.Code, lang)
			}
		}
	}
}
