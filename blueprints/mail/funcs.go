// Package mail is the mail blueprint's code: the brand and the words of
// the emails' layout, and its checks.
package mail

import (
	"embed"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// The words of the emails' layout, one file per locale.
//
//go:embed words/*.json
var wordFiles embed.FS

// TemplateFuncs are the mail blueprint's template functions.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"mailBrand":    brandOf,
		"mailBrandDir": BrandDir,
		"mailWords":    Words,
		"mailColors":   func() []string { return ColorRoles },
		"mailLogo":     func(b *ui.Brand) string { f, _ := b.EmailLogo(); return f },
		"mailLogoType": func(b *ui.Brand) string { _, t := b.EmailLogo(); return t },
		"mailBase":     path.Base,
	}
}

// ColorRoles are the brand's colors an email's layout takes.
var ColorRoles = []string{"page", "surface", "ink", "muted", "line", "accent", "on_accent"}

// BrandDir is the brand folder mail's options (or a site's) name, brand
// without one: where the brand's Go package is.
func BrandDir(opts map[string]any) string {
	if dir, _ := opts["brand"].(string); strings.Trim(dir, "/") != "" {
		return strings.Trim(dir, "/")
	}
	return "brand"
}

// brandOf is the emails' brand, read from the project of app; nil without
// one (or when it cannot be read: the check says why).
func brandOf(app *project.App, opts map[string]any) *ui.Brand {
	if app == nil {
		return nil
	}
	b, err := ui.BrandOf(app.Pos.File, opts)
	if err != nil {
		return nil
	}
	return b
}

// Word is a word of the layout: its key and its text, with {app} for the
// app's name.
type Word struct {
	Key, Text string
}

// Words are the layout's words in a locale (en, pt_br), by key; English
// for a locale esquadro does not know.
func Words(locale string) []Word {
	data, err := wordFiles.ReadFile("words/" + locale + ".json")
	if err != nil {
		data, _ = wordFiles.ReadFile("words/en.json")
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		panic("mail: words/" + locale + ".json: " + err.Error())
	}
	out := make([]Word, 0, len(m))
	for k, v := range m {
		out = append(out, Word{Key: k, Text: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
