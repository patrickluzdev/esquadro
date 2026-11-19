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
  "colors": {"accent": "#2F6FEB", "on_accent": "#FFFFFF", "accent_edge": "#1E4FB8", "page": "#FAFAF9", "muted": "#6B7280", "sale": "#E5484D"},
  "fonts": {
    "display": {"family": "Display Sans", "files": {"200 800": "fonts/Display.ttf"}},
    "body": {"family": "Body Sans", "files": {"300 900": "fonts/Body.woff2"}}
  },
  "text": {"section": {"font": "display", "weight": 800, "size": 28, "line": 1.1, "tracking": -0.02}, "caption": {"size": 12, "color": "muted"}},
  "radius": {"md": 14},
  "buttons": "raised",
  "control": 44,
  "motion": {"enter": {"duration": 350, "curve": "cubic-bezier(.22,1.2,.36,1)"}},
  "states": {"not_found": "states/lost.svg"},
  "identity": {"logo": "logo.svg", "icons": "icons", "theme_color": "#2F6FEB"}
}`

var goodFiles = []string{"fonts/Display.ttf", "fonts/Body.woff2", "states/lost.svg", "logo.svg", "icons/favicon.ico", "icons/site.webmanifest", "icons/.DS_Store"}

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
	if b.Color("accent") != "#2F6FEB" || b.Color("ink") == "" || b.Color("nope") != "" {
		t.Errorf("Color: a role's, a default, or nothing")
	}
	if b.ThemeColor() != "#2F6FEB" {
		t.Errorf("ThemeColor = %s", b.ThemeColor())
	}
}

func TestBrandProblems(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"color":    {`"page": "#FAFAF9"`, `"page": "beige"`, `colors.page "beige" is not a color`},
		"name":     {`"sale": "#E5484D"`, `"Sale": "#E5484D"`, `colors.Sale: a color's name is lowercase`},
		"font":     {`"display": {"family"`, `"title": {"family"`, `fonts.title: the fonts are display and body`},
		"weight":   {`"200 800"`, `"bold"`, `"bold" is not a weight`},
		"file":     {`fonts/Body.woff2`, `fonts/Missing.woff2`, `fonts.body.files.300 900: fonts/Missing.woff2 is not in brand`},
		"outside":  {`logo.svg`, `../logo.svg`, `identity.logo "../logo.svg" is not a file of brand`},
		"role":     {`"section": {`, `"Section": {`, `text.Section: a role's name is lowercase`},
		"size":     {`"caption": {"size": 12`, `"caption": {"size": 0`, `text.caption has no size`},
		"radius":   {`"md": 14`, `"xl": 14`, `radius.xl: the radii are sm, md and lg`},
		"buttons":  {`"raised"`, `"round"`, `buttons "round" is flat or raised`},
		"state":    {`"not_found":`, `"lost":`, `states.lost is not a state`},
		"icons":    {`"icons": "icons"`, `"icons": "logo.svg"`, `identity.icons: logo.svg is not a folder of brand`},
		"theme":    {`"theme_color": "#2F6FEB"`, `"theme_color": "green"`, `identity.theme_color "green" is not a color`},
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
		`@font-face { font-family: "Display Sans"; src: url("/site/brand/fonts/Display.ttf") format("truetype"); font-weight: 200 800;`,
		`src: url("/site/brand/fonts/Body.woff2") format("woff2")`,
		"--brand-accent: #2F6FEB;",
		"--brand-ink: #0a0a0a;", // a role the theme leaves out has its default
		"--brand-sale: #E5484D;",
		"--color-sale: var(--brand-sale);",
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
	// The roles come in the order of their names, whatever names they are.
	if strings.Index(css, ".t-caption") > strings.Index(css, ".t-section") {
		t.Error("text roles are not in order")
	}
	for _, catalog := range []string{".price", ".photo", "--brand-strike"} {
		if strings.Contains(css, catalog) {
			t.Errorf("CSS has %s, which is a project's", catalog)
		}
	}
	for _, want := range []string{} {
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

// An email shows a PNG, JPEG or GIF logo, never an SVG, in the brand's
// fonts by name, with the system's after them.
func TestBrandForEmail(t *testing.T) {
	b, err := BrandOf(brandProject(t, goodTheme, goodFiles...), map[string]any{"brand": "brand"})
	if err != nil {
		t.Fatal(err)
	}
	if f, typ := b.EmailLogo(); f != "" || typ != "" {
		t.Errorf("an SVG logo is no email's: %s %s", f, typ)
	}
	if got := b.FontStack("display"); got != "'Display Sans', -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif" {
		t.Errorf("FontStack(display) = %s", got)
	}
	png := strings.Replace(goodTheme, `"logo": "logo.svg"`, `"logo": "logo.svg", "logo_email": "logo.png"`, 1)
	b, _ = BrandOf(brandProject(t, png, append(goodFiles, "logo.png")...), map[string]any{"brand": "brand"})
	if f, typ := b.EmailLogo(); f != "logo.png" || typ != "image/png" {
		t.Errorf("EmailLogo = %s %s, want logo_email", f, typ)
	}
	if p := b.Problems(); len(p) != 0 {
		t.Errorf("a PNG logo_email has problems: %v", p)
	}
	jpeg := strings.Replace(goodTheme, `"logo": "logo.svg"`, `"logo": "logo.JPG"`, 1)
	b, _ = BrandOf(brandProject(t, jpeg, append(goodFiles, "logo.JPG")...), map[string]any{"brand": "brand"})
	if f, typ := b.EmailLogo(); f != "logo.JPG" || typ != "image/jpeg" {
		t.Errorf("EmailLogo = %s %s, want the JPEG logo", f, typ)
	}
	svg := strings.Replace(png, "logo.png", "logo-mail.svg", 1)
	b, _ = BrandOf(brandProject(t, svg, append(goodFiles, "logo-mail.svg")...), map[string]any{"brand": "brand"})
	if got := strings.Join(b.Problems(), "\n"); !strings.Contains(got, "identity.logo_email logo-mail.svg is not a PNG, JPEG or GIF") {
		t.Errorf("an SVG logo_email passes: %s", got)
	}
	b, _ = BrandOf(brandProject(t, `{"fonts": {"body": {"family": "Body", "fallback": "Georgia, serif"}}}`), map[string]any{"brand": "brand"})
	if got := b.FontStack("display"); got != "'Body', Georgia, serif" {
		t.Errorf("display falls back to body: %s", got)
	}
	b, _ = BrandOf(brandProject(t, `{}`), map[string]any{"brand": "brand"})
	if got := b.FontStack("body"); !strings.HasPrefix(got, "-apple-system") {
		t.Errorf("without fonts, the system's: %s", got)
	}
}
