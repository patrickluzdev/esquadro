package spa

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// hexColor is a color as the spa's options take it: #rgb or #rrggbb.
var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// ValidColor reports whether s is a color the theme takes.
func ValidColor(s string) bool { return hexColor.MatchString(s) }

// rgb is a color's channels, 0 to 255.
func rgb(hex string) (r, g, b float64) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	n, _ := strconv.ParseUint(h, 16, 32)
	return float64(n >> 16 & 0xff), float64(n >> 8 & 0xff), float64(n & 0xff)
}

// luminance is a color's relative luminance, as WCAG measures it: 0 black,
// 1 white.
func luminance(hex string) float64 {
	r, g, b := rgb(hex)
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the contrast ratio of two colors, 1 to 21.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// OnColor is the text that reads on a color: white, as long as it reads
// (3:1, the least for a button's bold text), else near black.
func OnColor(hex string) string {
	if !ValidColor(hex) || contrast(hex, "#ffffff") >= 3 {
		return "#ffffff"
	}
	return "#171717"
}

// DarkPrimary is the primary color of the dark theme: the one given, or
// the light theme's, unless it would not stand out on the dark cards
// (#262626), where a light gray does.
func DarkPrimary(primary, dark string) string {
	if ValidColor(dark) {
		return strings.ToLower(dark)
	}
	if ValidColor(primary) && contrast(primary, "#262626") >= 2 {
		return strings.ToLower(primary)
	}
	return "#e5e5e5"
}

// ColorError says what is wrong with an option's color, or nothing.
func ColorError(option, value string) string {
	if value == "" || ValidColor(value) {
		return ""
	}
	return fmt.Sprintf("%s %q is not a color: #rgb or #rrggbb, as #2a85ff", option, value)
}
