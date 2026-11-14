package diag

import (
	"strings"
	"unicode/utf8"
)

// Suggest returns the candidate closest to name, or "" when none is close
// enough to be a plausible typo. Comparison is case-insensitive.
func Suggest(name string, candidates []string) string {
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	maxDist := utf8.RuneCountInString(name) / 3
	if maxDist < 1 {
		maxDist = 1
	}
	if maxDist > 3 {
		maxDist = 3
	}
	best, bestDist := "", maxDist+1
	for _, c := range candidates {
		if c == name {
			continue
		}
		d := levenshtein(lower, strings.ToLower(c))
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// levenshtein computes the edit distance between a and b, counting runes.
// Swapping two adjacent runes counts as one edit (optimal string alignment),
// since that is the most common typo ("enmu" for "enum").
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
