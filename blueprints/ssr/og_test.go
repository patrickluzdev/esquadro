package ssr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
)

func TestOGFontPicksAFileItDraws(t *testing.T) {
	b := &ui.Brand{Theme: ui.BrandTheme{Fonts: map[string]ui.BrandFont{
		"display": {Files: map[string]string{"400": "D-Regular.ttf", "700": "D-Bold.ttf", "800": "D-Black.woff2"}},
		"body":    {Files: map[string]string{"100 900": "B-Variable.ttf", "400": "B-Regular.woff2"}},
	}}}
	if got := OGFont(b, "display"); got != "D-Bold.ttf" {
		t.Errorf("display: %s, want the bold TrueType", got)
	}
	if got := OGFont(b, "body"); got != "B-Variable.ttf" {
		t.Errorf("body: %s, want the variable TrueType (WOFF2 is not drawn)", got)
	}
	b.Theme.Fonts["body"] = ui.BrandFont{Files: map[string]string{"400": "B.woff2"}}
	if got := OGFont(b, "body"); got != "" {
		t.Errorf("only WOFF2: %s, want none", got)
	}
	if OGFont(nil, "display") != "" || OGFont(b, "mono") != "" {
		t.Error("a font of no brand, or of no name")
	}
}

func TestOGColorsAndLogo(t *testing.T) {
	plain := OGColors(nil, map[string]any{"primary": "#123456"})
	if plain["accent"] != "#123456" || plain["page"] == "" || plain["ink"] == "" {
		t.Errorf("without a brand: %v", plain)
	}
	b := &ui.Brand{Theme: ui.BrandTheme{Colors: map[string]string{"page": "#000000", "sale": "#ff0000"}}}
	branded := OGColors(b, map[string]any{"primary": "#123456"})
	if branded["page"] != "#000000" || branded["sale"] != "#ff0000" || branded["accent"] == "#123456" {
		t.Errorf("with a brand: %v", branded)
	}
	b.Theme.Identity = ui.BrandIdentity{Logo: "logo.svg", Symbol: "symbol.png"}
	if got := OGLogo(b); got != "symbol.png" {
		t.Errorf("logo %q, want the symbol: an SVG is not drawn", got)
	}
	b.Theme.Identity = ui.BrandIdentity{Logo: "logo.svg"}
	if got := OGLogo(b); got != "" {
		t.Errorf("logo %q, want none", got)
	}
}

func TestOGThemeHashChangesWithTheBrand(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "brand"), 0o755)
	font := filepath.Join(root, "brand", "D.ttf")
	os.WriteFile(font, []byte("one"), 0o644)
	b := &ui.Brand{Root: root, Dir: "brand", Theme: ui.BrandTheme{Fonts: map[string]ui.BrandFont{"display": {Files: map[string]string{"700": "D.ttf"}}}}}
	first := OGThemeHash(b, nil)
	if again := OGThemeHash(b, nil); again != first || len(first) != 8 {
		t.Fatalf("%s, then %s", first, again)
	}
	os.WriteFile(font, []byte("two"), 0o644)
	if OGThemeHash(b, nil) == first {
		t.Error("a new font file draws nothing again")
	}
	b.Theme.Colors = map[string]string{"accent": "#000000"}
	if OGThemeHash(b, nil) == first {
		t.Error("a new color draws nothing again")
	}
	if OGThemeHash(nil, map[string]any{"primary": "#111111"}) == OGThemeHash(nil, map[string]any{"primary": "#222222"}) {
		t.Error("a new primary draws nothing again")
	}
}
