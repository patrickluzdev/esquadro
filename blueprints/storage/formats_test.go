package storage

import (
	"slices"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"5MB": 5 << 20, "500kb": 500 << 10, "1GB": 1 << 30, "2048": 2048, " 10 MB ": 10 << 20} {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "MB", "-1MB", "0", "8 megas", "1.5MB"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) took it", in)
		}
	}
}

func TestKnown(t *testing.T) {
	for _, f := range []string{"png", "PDF", "docx", "application/x-custom"} {
		if !Known(f) {
			t.Errorf("%s should be known", f)
		}
	}
	for _, f := range []string{"banana", "image/", "a b/c", ""} {
		if Known(f) {
			t.Errorf("%s should not be known", f)
		}
	}
}

func TestRulesOf(t *testing.T) {
	opts := map[string]any{
		"image_types": []any{"png", "jpeg", "webp"}, "image_max": "10MB",
		"file_types": []any{}, "file_max": "25MB", "url_minutes": int64(60),
	}
	photo := &project.Field{Type: project.TypeImage}
	r := RulesOf(photo, opts)
	if r.Max != 10<<20 || !slices.Equal(r.Types, []string{"image/png", "image/jpeg", "image/webp"}) || !r.Inline || r.URLMinutes != 60 {
		t.Errorf("an image by the blueprint's rules: %+v", r)
	}

	manual := &project.Field{Type: project.TypeFile, Ann: project.Ann{"storage": {"max": "20MB", "types": []any{"pdf"}, "url_minutes": int64(5)}}}
	r = RulesOf(manual, opts)
	if r.Max != 20<<20 || !slices.Equal(r.Types, []string{"application/pdf"}) || r.Inline || r.URLMinutes != 5 {
		t.Errorf("a file by its own rules: %+v", r)
	}

	plain := &project.Field{Type: project.TypeFile}
	if r := RulesOf(plain, opts); len(r.Types) != 0 || r.Inline || r.Max != 25<<20 {
		t.Errorf("a file of any format, downloaded: %+v", r)
	}

	// SVG can run script: never opened in the browser, even when asked.
	svg := &project.Field{Type: project.TypeImage, Ann: project.Ann{"storage": {"types": []any{"svg"}, "inline": true}}}
	if r := RulesOf(svg, opts); r.Inline {
		t.Errorf("an SVG opens in the browser: %+v", r)
	}
	pdf := &project.Field{Type: project.TypeFile, Ann: project.Ann{"storage": {"types": []any{"pdf"}, "inline": true}}}
	if r := RulesOf(pdf, opts); !r.Inline {
		t.Errorf("a PDF asked inline is not: %+v", r)
	}
}
