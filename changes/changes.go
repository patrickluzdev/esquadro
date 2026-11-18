// Package changes holds what each version of esquadro changes for the
// projects it generates: one file per version, vX.Y.Z.md, with what breaks
// and how to migrate. esquadro upgrade shows those between a project's
// version and its own.
package changes

import (
	"embed"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

//go:embed v*.md
var files embed.FS

// Note is what a version changes.
type Note struct {
	Version string
	Text    string
}

// Between are the notes of the versions after from and up to to, oldest
// first; to "" (a development build) has no end.
func Between(from, to string) []Note {
	entries, _ := files.ReadDir(".")
	var out []Note
	for _, e := range entries {
		v := strings.TrimSuffix(e.Name(), ".md")
		if !semver.IsValid(v) {
			continue
		}
		if from != "" && semver.IsValid(from) && semver.Compare(v, from) <= 0 {
			continue
		}
		if to != "" && semver.IsValid(to) && semver.Compare(v, to) > 0 {
			continue
		}
		text, err := files.ReadFile(e.Name())
		if err != nil {
			continue
		}
		out = append(out, Note{Version: v, Text: string(text)})
	}
	sort.Slice(out, func(i, j int) bool { return semver.Compare(out[i].Version, out[j].Version) < 0 })
	return out
}
