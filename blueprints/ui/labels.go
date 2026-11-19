package ui

import (
	"strings"
	"unicode"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Humanize is a name as people read it: due_on is "Due on".
func Humanize(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Lucide is the component of a Lucide icon's name: shopping-cart is
// ShoppingCart; Circle without one.
func Lucide(name any) string {
	s, _ := name.(string)
	if s == "" {
		return "Circle"
	}
	var b strings.Builder
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == ' ' }) {
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// colorWords guess a value's color from the words of its name.
var colorWords = []struct {
	color string
	words []string
}{
	{"red", []string{"CANCEL", "FAIL", "ERROR", "REJECT", "BLOCK", "BAN", "DENI", "EXPIRE", "OVERDUE", "LOST", "DELETED", "REFUND"}},
	{"amber", []string{"PENDING", "DRAFT", "WAIT", "REVIEW", "HOLD", "PAUS", "PROCESS", "PARTIAL", "TRIAL", "SCHEDUL", "LOW"}},
	{"green", []string{"ACTIVE", "ENABLE", "PAID", "DONE", "COMPLETE", "APPROV", "PUBLISH", "SUCCESS", "OPEN", "ONLINE", "VERIFIED", "AVAILABLE", "DELIVERED", "WON", "IN_STOCK"}},
	{"blue", []string{"NEW", "SENT", "SHIPPED", "PROGRESS", "RUNNING", "CONFIRM"}},
}

// EnumColor is the color of an enum value's dot: its own (@<spa>(color
// ...)), or a guess from its name; gray when none fits.
func EnumColor(v *project.EnumValue, ann map[string]any) string {
	if c, ok := ann["color"].(string); ok && c != "" {
		return c
	}
	name := strings.ToUpper(v.Raw)
	// INACTIVE, DISABLED, UNPAID: a negation is not the word it negates.
	if strings.HasPrefix(name, "IN") && strings.Contains(name, "ACTIVE") || strings.HasPrefix(name, "DIS") || strings.HasPrefix(name, "UN") || strings.HasPrefix(name, "NOT_") {
		return "gray"
	}
	for _, c := range colorWords {
		for _, w := range c.words {
			if strings.Contains(name, w) {
				return c.color
			}
		}
	}
	return "gray"
}

// unaccent takes the accents off the letters of Latin languages.
var unaccent = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// Slug is a dashboard's name in its address: Sales and marketing,
// sales-and-marketing.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range unaccent.Replace(strings.ToLower(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
	}
	return b.String()
}
