package ssr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// OGColors are the colors a site's images are drawn with, by role: its
// brand's, its own colors too; without a brand, the neutral look with the
// site's primary as the accent.
func OGColors(b *ui.Brand, opts map[string]any) map[string]string {
	out := map[string]string{}
	if b == nil {
		b = &ui.Brand{}
		if primary, _ := opts["primary"].(string); primary != "" {
			b.Theme.Colors = map[string]string{"accent": primary}
		}
	}
	for _, role := range ui.ColorRoles {
		if c := b.Color(role); c != "" {
			out[role] = c
		}
	}
	for name, c := range b.Theme.Colors {
		out[name] = c
	}
	return out
}

// OGThemeHash tells apart what a site's images are drawn with: its colors,
// and the content of the font and logo files they read. It is part of the
// name each image is kept under, so a change to the brand draws them
// again.
func OGThemeHash(b *ui.Brand, opts map[string]any) string {
	h := sha256.New()
	colors := OGColors(b, opts)
	roles := slices.Sorted(maps.Keys(colors))
	for _, role := range roles {
		fmt.Fprintf(h, "%s=%s\n", role, colors[role])
	}
	if b != nil {
		for _, file := range []string{OGFont(b, "display"), OGFont(b, "body"), OGLogo(b)} {
			fmt.Fprintf(h, "file %s\n", file)
			if file == "" {
				continue
			}
			if data, err := os.ReadFile(filepath.Join(b.Root, b.Dir, filepath.FromSlash(file))); err == nil {
				h.Write(data)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// imageFonts are the font files the images blueprint draws with.
var imageFonts = map[string]bool{".ttf": true, ".otf": true, ".ttc": true, ".otc": true}

// OGFont is the file of a brand's font (display or body) that a site's
// images are drawn with: a TrueType or OpenType one (WOFF and WOFF2 are
// not drawn), of the weight nearest a title's (700) or a text's (400); a
// static one of that weight before a variable one, which is drawn in its
// default instance. "" for none.
func OGFont(b *ui.Brand, name string) string {
	if b == nil {
		return ""
	}
	f, ok := b.Theme.Fonts[name]
	if !ok {
		return ""
	}
	want := 400.0
	if name == "display" {
		want = 700
	}
	best, score := "", math.Inf(1)
	for weight, file := range f.Files {
		if !imageFonts[strings.ToLower(path.Ext(file))] {
			continue
		}
		lo, hi, variable := strings.Cut(weight, " ")
		if !variable {
			hi = lo
		}
		l, _ := strconv.ParseFloat(lo, 64)
		h, _ := strconv.ParseFloat(hi, 64)
		s := 0.0
		switch {
		case want < l:
			s = l - want
		case want > h:
			s = want - h
		}
		if variable {
			s += 50 // its default instance is likely another weight
		}
		if s < score || (s == score && file < best) {
			best, score = file, s
		}
	}
	return best
}

// OGLogo is the brand's logo a site's images show, a file of its folder:
// the logo, or the symbol, when it is a picture the images blueprint
// reads (PNG, JPEG, GIF, WebP; not SVG); "" for none.
func OGLogo(b *ui.Brand) string {
	if b == nil {
		return ""
	}
	for _, file := range []string{b.Theme.Identity.Logo, b.Theme.Identity.Symbol} {
		switch strings.ToLower(path.Ext(file)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
			return file
		}
	}
	return ""
}

// ogParents are the records of its own module an entity needs first, for
// the generated test of its images.
func ogParents(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.OwnsForeignKey() && r.Required && r.Target.Module == e.Module {
			out = append(out, r)
		}
	}
	return out
}

// ogSample reports whether the generated test can write a record of e
// itself, to draw its image: valid values of its fields, and records it
// needs that need none themselves.
func ogSample(e *project.Entity) bool {
	if !store.CanSample(e) {
		return false
	}
	for _, r := range ogParents(e) {
		if r.Target == e || len(ogParents(r.Target)) > 0 || !store.CanSample(r.Target) {
			return false
		}
	}
	return true
}

// ogSampleImports are what the generated test of a site's images imports
// to write records of the entities given (decimal, time...).
func ogSampleImports(pages []*Page) []string {
	var fields []*project.Field
	for _, pg := range pages {
		if !pg.OG || !ogSample(pg.Entity) {
			continue
		}
		fields = append(fields, pg.Entity.Fields...)
		for _, r := range ogParents(pg.Entity) {
			fields = append(fields, r.Target.Fields...)
		}
	}
	return store.SampleImports(fields)
}

// ogTested are the entities with images on a site whose generated test
// writes a record of its own: public, and of values esquadro can write.
func ogTested(pages []*Page) []*project.Entity {
	var out []*project.Entity
	for _, pg := range pages {
		if pg.OG && ogSample(pg.Entity) && ogPublic(pg.Entity) {
			out = append(out, pg.Entity)
		}
	}
	return out
}

// ogPublic reports whether everyone may read an entity's records: its
// images are then kept by shared caches too.
func ogPublic(e *project.Entity) bool {
	return slices.Contains(ui.Strings(e.Ann["security"]["read"]), "public")
}

// ogPackages are the modules of entities, as Go imports them.
func ogPackages(appModule string, entities []*project.Entity) []string {
	var out []string
	for _, e := range entities {
		if p := appModule + "/internal/" + e.Module.Snake; !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
