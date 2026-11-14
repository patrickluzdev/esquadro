package project

import "strings"

var irregularEN = map[string]string{
	"person": "people", "child": "children", "man": "men", "woman": "women",
	"mouse": "mice", "goose": "geese", "tooth": "teeth", "foot": "feet",
	"ox": "oxen", "datum": "data", "index": "indices", "matrix": "matrices",
	"vertex": "vertices", "criterion": "criteria", "analysis": "analyses",
	"leaf": "leaves", "life": "lives", "knife": "knives", "wife": "wives",
	"half": "halves", "wolf": "wolves", "shelf": "shelves", "thief": "thieves",
	"calf": "calves", "loaf": "loaves",
	"hero": "heroes", "potato": "potatoes", "tomato": "tomatoes", "echo": "echoes",
	"quiz": "quizzes",
}

var uncountableEN = map[string]bool{
	"news": true, "data": true, "information": true, "equipment": true,
	"series": true, "species": true, "fish": true, "sheep": true,
	"metadata": true, "feedback": true, "software": true, "hardware": true,
	"media": true, "staff": true, "settings": true,
}

// pluralize returns the English plural of a single lower-case word. It covers
// the regular rules and common irregular words; anything else can be fixed
// with the entity option (plural "...").
func pluralize(w string) string {
	if p, ok := irregularEN[w]; ok {
		return p
	}
	if uncountableEN[w] {
		return w
	}
	switch {
	case hasAnySuffix(w, "s", "x", "z", "ch", "sh"):
		return w + "es"
	case strings.HasSuffix(w, "y") && len(w) > 1 && !isVowel(w[len(w)-2]):
		return w[:len(w)-1] + "ies"
	}
	return w + "s"
}

func isVowel(b byte) bool { return strings.IndexByte("aeiou", b) >= 0 }

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, x := range suffixes {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}
