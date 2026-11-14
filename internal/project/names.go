package project

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Names holds every spelling of a name that templates need.
type Names struct {
	Raw    string // as written in the .esq file, e.g. "orderItem"
	Label  string // human-readable, e.g. "Order item"
	Pascal string // OrderItem, UserID
	Camel  string // orderItem, userID
	Snake  string // order_item, user_id
	Kebab  string // order-item, user-id
}

// commonInitialisms are kept upper-case in Pascal and Camel forms, following
// Go naming conventions (userID, not userId).
var commonInitialisms = map[string]bool{
	"api": true, "ascii": true, "cpu": true, "css": true, "dns": true,
	"eof": true, "guid": true, "html": true, "http": true, "https": true,
	"id": true, "ip": true, "json": true, "jwt": true, "rpc": true,
	"sql": true, "ssh": true, "tcp": true, "tls": true, "ttl": true,
	"udp": true, "ui": true, "uid": true, "uri": true, "url": true,
	"utf8": true, "uuid": true, "xml": true,
}

// splitWords splits an identifier into lower-case words at "_", "-", spaces
// and case changes: "orderItem", "order_item" and "OrderItem" all give
// [order item]; "HTTPServer" gives [http server].
func splitWords(s string) []string {
	runes := []rune(s)
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	for i, r := range runes {
		if r == '_' || r == '-' || r == ' ' {
			flush()
			continue
		}
		if i > 0 && len(cur) > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// namesFromWords builds every form from lower-case words. It returns
// ok=false when a word is not plain ASCII: generated code uses these names in
// Go, SQL and file paths.
func namesFromWords(raw string, words []string) (Names, bool) {
	for _, w := range words {
		for _, r := range w {
			if r >= utf8.RuneSelf {
				return Names{}, false
			}
		}
	}
	n := Names{Raw: raw}
	var pascal, camel strings.Builder
	for i, w := range words {
		p := capitalize(w)
		if commonInitialisms[w] {
			p = strings.ToUpper(w)
		}
		pascal.WriteString(p)
		if i == 0 {
			camel.WriteString(w)
		} else {
			camel.WriteString(p)
		}
	}
	n.Pascal = pascal.String()
	n.Camel = camel.String()
	n.Snake = strings.Join(words, "_")
	n.Kebab = strings.Join(words, "-")
	n.Label = capitalize(strings.Join(words, " "))
	return n, true
}

// makeNames builds the names for an identifier.
func makeNames(raw string) (Names, bool) {
	return namesFromWords(raw, splitWords(raw))
}

// makePlural builds the plural names for an identifier by pluralizing its last
// word. Raw is the plural words joined with "_" ("order_items").
func makePlural(raw string) (Names, bool) {
	words := splitWords(raw)
	if len(words) == 0 {
		return Names{Raw: raw}, true
	}
	i := len(words) - 1
	words[i] = pluralize(words[i])
	return namesFromWords(strings.Join(words, "_"), words)
}

func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
