package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/project"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// The kinds of .esq file, which decide how a file is checked.
const (
	projectFile  = iota // in a project's esquadro/ folder: checked with its project
	manifestFile        // a blueprint's blueprint.esq: checked on its own
	looseFile           // anywhere else: only its syntax
)

// kindOf tells what an .esq file is from its path.
func kindOf(path string) int {
	switch {
	case filepath.Base(path) == blueprint.ManifestFile:
		return manifestFile
	case filepath.Base(filepath.Dir(path)) == workspace.Dir:
		return projectFile
	}
	return looseFile
}

// checkAll checks everything the open documents belong to: each one's
// project, manifest or syntax. It returns the diagnostics by file, with an
// entry, maybe empty, for every file checked.
func checkAll(docs map[string]string, cacheDir string) map[string][]diag.Diagnostic {
	out := map[string][]diag.Diagnostic{}
	add := func(d diag.Diagnostic) {
		if !filepath.IsAbs(d.Pos.File) {
			return // inside a built-in blueprint: not a file of the user's
		}
		out[d.Pos.File] = append(out[d.Pos.File], d)
	}
	checked := func(path string) {
		if out[path] == nil {
			out[path] = []diag.Diagnostic{}
		}
	}
	overlay := map[string][]byte{}
	for path, src := range docs {
		if kindOf(path) == manifestFile {
			overlay[path] = []byte(src)
		}
	}
	roots := map[string]bool{}
	for path, src := range docs {
		switch kindOf(path) {
		case projectFile:
			roots[filepath.Dir(filepath.Dir(path))] = true
		case manifestFile:
			checked(path)
			_, diags := blueprint.ParseManifest(path, []byte(src))
			for _, d := range diags {
				add(d)
			}
		case looseFile:
			checked(path)
			_, diags := dsl.Parse(path, []byte(src))
			for _, d := range diags {
				add(d)
			}
		}
	}
	for root := range roots {
		files, diags := checkProject(root, docs, overlay, cacheDir)
		for _, f := range files {
			checked(f)
		}
		for _, d := range diags {
			add(d)
		}
	}
	for path, list := range out {
		out[path] = dedupe(list)
	}
	return out
}

// checkProject checks the project at root as esquadro check does, with the
// open documents in place of the disk and blueprints from git only from the
// cache. It returns the project's files and the diagnostics.
func checkProject(root string, docs map[string]string, overlay map[string][]byte, cacheDir string) ([]string, diag.List) {
	model := filepath.Join(root, workspace.Dir)
	paths, _ := filepath.Glob(filepath.Join(model, "*.esq"))
	// Files open in the editor but never saved belong to the project too.
	for path := range docs {
		if filepath.Dir(path) == model && !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var files []string
	var sources []project.Source
	for _, p := range paths {
		if filepath.Base(p) == blueprint.ManifestFile {
			continue
		}
		src, ok := docs[p]
		if !ok {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			src = string(data)
		}
		files = append(files, p)
		sources = append(sources, project.Source{Name: p, Src: []byte(src)})
	}
	if len(sources) == 0 {
		return nil, nil
	}
	p, diags := project.Load(sources...)
	if diags.HasErrors() {
		return files, diags
	}
	pinned := map[string]string{}
	if lock, err := workspace.ReadLock(root); err == nil {
		for _, b := range lock.Blueprints {
			if b.Source != "" && b.Commit != "" {
				pinned[b.Source] = b.Commit
			}
		}
	}
	l := &blueprint.Loader{
		Builtin:    blueprints.FS,
		ProjectDir: model,
		// The editor checks the model, not that esquadro implements its
		// own hooks.
		HasBuiltinHook: func(string, string) bool { return true },
		Git:            &blueprint.Git{CacheDir: cacheDir, Pinned: pinned, Offline: true},
		Overlay:        overlay,
	}
	_, more := blueprint.Resolve(p, l)
	return files, append(diags, more...)
}

// dedupe drops repeated diagnostics (a manifest checked on its own and by a
// project) and sorts the rest by position.
func dedupe(list []diag.Diagnostic) []diag.Diagnostic {
	seen := map[string]bool{}
	out := make([]diag.Diagnostic, 0, len(list))
	for _, d := range list {
		key := fmt.Sprintf("%d:%d:%d:%s", d.Pos.Line, d.Pos.Col, d.Severity, d.Message)
		if !seen[key] {
			seen[key] = true
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pos.Line != out[j].Pos.Line {
			return out[i].Pos.Line < out[j].Pos.Line
		}
		return out[i].Pos.Col < out[j].Pos.Col
	})
	return slices.Clip(out)
}
