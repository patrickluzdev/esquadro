package main

import (
	"bytes"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/starters"
)

// starterNames are the starters, in the order esquadro new lists them.
func starterNames() []string {
	return []string{"api", "worker", "cli", "platform"}
}

// starterAbout is the line that says what a starter is for.
func starterAbout(name string) string {
	about, err := fs.ReadFile(starters.FS, path.Join(name, "about"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(about))
}

// starterFiles are the .esq files of a project made from a starter.
func starterFiles(name, project, module string, git bool) (map[string]string, error) {
	files := map[string]string{}
	entries, err := fs.ReadDir(starters.FS, name)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		src, err := fs.ReadFile(starters.FS, path.Join(name, e.Name()))
		if err != nil {
			return nil, err
		}
		switch {
		case e.Name() == "app.esq.tmpl" || e.Name() == "platform.esq.tmpl":
			t, err := template.New(e.Name()).Option("missingkey=error").Parse(string(src))
			if err != nil {
				return nil, err
			}
			var b bytes.Buffer
			if err := t.Execute(&b, map[string]any{
				"Name": strings.ReplaceAll(project, "-", "_"), "Label": label(project), "Module": module, "Git": git,
			}); err != nil {
				return nil, err
			}
			files[strings.TrimSuffix(e.Name(), ".tmpl")] = b.String()
		case strings.HasSuffix(e.Name(), ".esq"):
			files[e.Name()] = string(src)
		}
	}
	return files, nil
}

var blueprintLine = regexp.MustCompile(`(?m)^\s*blueprint ([a-z_][a-z0-9_]*)\b`)

// starterBlueprints are the blueprints a starter's app or platform uses,
// without git.
func starterBlueprints(name string) []string {
	src, err := fs.ReadFile(starters.FS, path.Join(name, "app.esq.tmpl"))
	if err != nil {
		if src, err = fs.ReadFile(starters.FS, path.Join(name, "platform.esq.tmpl")); err != nil {
			return nil
		}
	}
	var bps []string
	for _, m := range blueprintLine.FindAllStringSubmatch(string(src), -1) {
		if m[1] != "git" {
			bps = append(bps, m[1])
		}
	}
	return bps
}
