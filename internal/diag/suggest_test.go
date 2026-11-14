package diag

import "testing"

func TestSuggest(t *testing.T) {
	cands := []string{"app", "enum", "entity", "Category", "Product"}
	tests := []struct{ in, want string }{
		{"entiy", "entity"},
		{"Categry", "Category"},
		{"category", "Category"},
		{"enmu", "enum"},
		{"xyz", ""},
		{"Customer", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := Suggest(tt.in, cands); got != tt.want {
			t.Errorf("Suggest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLevenshteinCountsRunes(t *testing.T) {
	if d := levenshtein("naïve", "naive"); d != 1 {
		t.Fatalf("got %d, want 1", d)
	}
}
