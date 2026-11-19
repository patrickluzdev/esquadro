package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// brandProject writes a project whose brand folder has theme and the files
// it names, and answers the path of its app.esq.
func brandProject(t *testing.T, theme string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "esquadro"), 0o755)
	app := filepath.Join(root, "esquadro", "app.esq")
	os.WriteFile(app, []byte("app shop {}\n"), 0o644)
	for _, f := range append(files, "theme.json") {
		p := filepath.Join(root, "brand", filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(root, "brand", "theme.json"), []byte(theme), 0o644)
	return app
}

const goodTheme = `{
  "colors": {"accent": "#A8DC50", "on_accent": "#1D2019", "accent_edge": "#86B33E", "page": "#F7F7F3", "muted": "#6A6F63", "casa": "#3CC8B4"},
  "fonts": {
    "display": {"family": "Bricolage Grotesque", "files": {"200 800": "fonts/Bricolage.ttf"}},
    "body": {"family": "Figtree", "files": {"300 900": "fonts/Figtree.woff2"}}
  },
  "text": {"section": {"font": "display", "weight": 800, "size": 28, "line": 1.1, "tracking": -0.02}, "caption": {"size": 12, "color": "muted"}},
  "radius": {"md": 14},
  "buttons": "raised",
  "control": 44,
  "motion": {"enter": {"duration": 350, "curve": "cubic-bezier(.22,1.2,.36,1)"}},
  "states": {"not_found": "states/lost.svg"},
  "identity": {"logo": "logo.svg", "icons": "icons", "theme_color": "#A8DC50"}
}`

var goodFiles = []string{"fonts/Bricolage.ttf", "fonts/Figtree.woff2", "states/lost.svg", "logo.svg", "icons/favicon.ico", "icons/site.webmanifest", "icons/.DS_Store"}

func TestBrandOf(t *testing.T) {
	app := brandProject(t, goodTheme, goodFiles...)
	if b, err := BrandOf(app, map[string]any{"brand": ""}); b != nil || err != nil {
		t.Errorf("no brand: %v, %v", b, err)
	}
	if _, err := BrandOf(app, map[string]any{"brand": "marca"}); err == nil {
		t.Error("a brand folder without theme.json is an error")
	}
	b, err := BrandOf(app, map[string]any{"brand": "brand/"})
	if err != nil {
		t.Fatal(err)
	}
	if p := b.Problems(); len(p) != 0 {
		t.Errorf("a good brand has problems: %v", p)
	}
	if got := strings.Join(b.IconFiles(), ","); got != "favicon.ico,site.webmanifest" {
		t.Errorf("IconFiles = %s, want the icons without hidden files", got)
	}
	if b.Color("accent") != "#A8DC50" || b.Color("ink") == "" || b.Color("nope") != "" {
		t.Errorf("Color: a role's, a default, or nothing")
	}
	if b.ThemeColor() != "#A8DC50" {
		t.Errorf("ThemeColor = %s", b.ThemeColor())
	}
}

func TestBrandProblems(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"color":    {`"page": "#F7F7F3"`, `"page": "beige"`, `colors.page "beige" is not a color`},
		"name":     {`"casa": "#3CC8B4"`, `"Casa": "#3CC8B4"`, `colors.Casa: a color's name is lowercase`},
		"font":     {`"display": {"family"`, `"title": {"family"`, `fonts.title: the fonts are display and body`},
		"weight":   {`"200 800"`, `"bold"`, `"bold" is not a weight`},
		"file":     {`fonts/Figtree.woff2`, `fonts/Missing.woff2`, `fonts.body.files.300 900: fonts/Missing.woff2 is not in brand`},
		"outside":  {`logo.svg`, `../logo.svg`, `identity.logo "../logo.svg" is not a file of brand`},
		"role":     {`"section": {`, `"headline": {`, `text.headline is not a role`},
		"size":     {`"caption": {"size": 12`, `"caption": {"size": 0`, `text.caption has no size`},
		"radius":   {`"md": 14`, `"xl": 14`, `radius.xl: the radii are sm, md and lg`},
		"buttons":  {`"raised"`, `"round"`, `buttons "round" is flat or raised`},
		"state":    {`"not_found":`, `"lost":`, `states.lost is not a state`},
		"icons":    {`"icons": "icons"`, `"icons": "logo.svg"`, `identity.icons: logo.svg is not a folder of brand`},
		"theme":    {`"theme_color": "#A8DC50"`, `"theme_color": "green"`, `identity.theme_color "green" is not a color`},
		"textfont": {`"font": "display", "weight": 800`, `"font": "serif", "weight": 800`, `text.section.font "serif" is display or body`},
	} {
		theme := strings.Replace(goodTheme, tc.from, tc.to, 1)
		if theme == goodTheme {
			t.Fatalf("%s: %q is not in the theme", name, tc.from)
		}
		b, err := BrandOf(brandProject(t, theme, goodFiles...), map[string]any{"brand": "brand"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Join(b.Problems(), "\n"); !strings.Contains(got, tc.want) {
			t.Errorf("%s: want %q in:\n%s", name, tc.want, got)
		}
	}
}

func TestBrandCSS(t *testing.T) {
	b, err := BrandOf(brandProject(t, goodTheme, goodFiles...), map[string]any{"brand": "brand"})
	if err != nil {
		t.Fatal(err)
	}
	css := b.CSS("/site/brand/")
	for _, want := range []string{
		`@font-face { font-family: "Bricolage Grotesque"; src: url("/site/brand/fonts/Bricolage.ttf") format("truetype"); font-weight: 200 800;`,
		`src: url("/site/brand/fonts/Figtree.woff2") format("woff2")`,
		"--brand-accent: #A8DC50;",
		"--brand-ink: #0a0a0a;", // a role the theme leaves out has its default
		"--brand-casa: #3CC8B4;",
		"--color-casa: var(--brand-casa);",
		"--primary: var(--brand-accent);",
		"--muted-foreground: var(--brand-muted);",
		"--brand-radius-md: 14px;",
		"--radius-md: 14px;",
		"--control: 44px;",
		"--ease-enter: cubic-bezier(.22,1.2,.36,1);",
		".t-section { font-family: var(--brand-font-display); font-weight: 800; font-size: 28px; line-height: 1.1; letter-spacing: -0.02em; }",
		".t-caption { font-family: var(--brand-font-body); font-weight: 400; font-size: 12px; line-height: 1.4; color: var(--brand-muted); }",
		".btn-primary { background: var(--brand-accent); color: var(--brand-on-accent); box-shadow: inset 0 -2px 0 var(--brand-accent-edge); }",
		".btn:active { transform: translateY(2px); }",
		".notice.hidden",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS lacks %q:\n%s", want, css)
		}
	}
	// A variable never names itself, which would leave it with no value.
	for _, m := range regexp.MustCompile(`--([a-z0-9-]+):\s*var\(--([a-z0-9-]+)\)`).FindAllStringSubmatch(css, -1) {
		if m[1] == m[2] {
			t.Errorf("--%s refers to itself", m[1])
		}
	}

	flat := strings.Replace(goodTheme, `"raised"`, `"flat"`, 1)
	b, _ = BrandOf(brandProject(t, flat, goodFiles...), map[string]any{"brand": "brand"})
	if css := b.CSS("/brand/"); strings.Contains(css, "inset 0 -2px") || !strings.Contains(css, "translateY(1px)") {
		t.Error("flat buttons have no edge")
	}
}
