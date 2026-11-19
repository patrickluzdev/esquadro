package ssr

import (
	"embed"
	"encoding/json"
	"sort"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
)

// The site's own words, beside those every frontend shares (ui's), one
// file per locale.
//
//go:embed words/*.json
var wordFiles embed.FS

// Words are a site's words in a locale: every frontend's, then the site's
// own, by key.
func Words(locale string) []ui.Word {
	out := ui.Words(locale)
	data, err := wordFiles.ReadFile("words/" + locale + ".json")
	if err != nil {
		return out
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		panic("ssr: words/" + locale + ".json: " + err.Error())
	}
	for k, v := range m {
		out = append(out, ui.Word{Key: k, Text: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
