package ui

import (
	"embed"
	"encoding/json"
	"sort"
)

// The words every frontend shares (signing in, the account, errors, empty
// lists, pages, fields), one file per locale; each frontend writes them in
// its own language (TypeScript, Go) beside its own words.
//
//go:embed words/*.json
var wordFiles embed.FS

// Word is a word of a frontend: its key and its text in a locale, with
// {placeholders}.
type Word struct {
	Key, Text string
}

// Words are the shared words in a locale (en, pt_br), by key; nil for a
// locale esquadro does not know.
func Words(locale string) []Word {
	data, err := wordFiles.ReadFile("words/" + locale + ".json")
	if err != nil {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		panic("ui: words/" + locale + ".json: " + err.Error())
	}
	out := make([]Word, 0, len(m))
	for k, v := range m {
		out = append(out, Word{Key: k, Text: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
