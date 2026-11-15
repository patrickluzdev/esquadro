package blueprint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Blueprint is a loaded blueprint: its manifest and its files.
type Blueprint struct {
	Manifest *Manifest
	// FS is rooted at the blueprint's folder.
	FS  fs.FS
	Ref *project.BlueprintRef
	// Location is where the blueprint came from, for messages:
	// "built-in", the folder path, or the git source with its commit.
	Location string
	// Dir is the blueprint's folder on disk, where its hooks run from;
	// empty for a built-in one.
	Dir string
	// Source and Commit are, for a blueprint from git, the source as the
	// app writes it and the commit used.
	Source, Commit string
}

// Loader finds blueprints.
type Loader struct {
	// Builtin holds the built-in blueprints, one folder per blueprint.
	Builtin fs.FS
	// ProjectDir is the folder local blueprint paths are relative to.
	ProjectDir string
	// HasBuiltinHook reports whether esquadro implements a built-in
	// blueprint's hook. When nil, built-in hooks must exist as files.
	HasBuiltinHook func(blueprint, path string) bool
	// Git fetches blueprints from git; without it, they are refused.
	Git *Git
	// Overlay holds manifests to read instead of the disk, by absolute
	// path: those open, maybe unsaved, in an editor.
	Overlay map[string][]byte
	// unfetched counts the blueprints from git the cache does not have
	// (Offline): annotations and requires cannot be checked without them.
	unfetched int
	// sources keeps every manifest read, for rendering diagnostics.
	sources map[string][]byte
}

// Source returns the contents of a manifest read by the loader, for
// rendering diagnostics, or nil.
func (l *Loader) Source(file string) []byte { return l.sources[file] }

// BuiltinNames lists the built-in blueprints.
func (l *Loader) BuiltinNames() []string {
	if l.Builtin == nil {
		return nil
	}
	entries, err := fs.ReadDir(l.Builtin, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := fs.Stat(l.Builtin, path.Join(e.Name(), ManifestFile)); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	return names
}

// Load loads the blueprint a project refers to. Diagnostics point at the
// reference in the project or into the blueprint's manifest.
func (l *Loader) Load(ref *project.BlueprintRef) (*Blueprint, diag.List) {
	var diags diag.List
	errAtRef := func(format string, args ...any) *diag.Diagnostic {
		return diags.Errorf(ref.RefSpan.Pos, ref.RefSpan.End, format, args...)
	}

	var (
		fsys           fs.FS
		location       string
		file           string
		dir            string
		source, commit string
	)
	switch ref.Source {
	case project.SourceBuiltin:
		if l.Builtin != nil {
			if sub, err := fs.Sub(l.Builtin, ref.Ref); err == nil {
				if _, err := fs.Stat(sub, ManifestFile); err == nil {
					fsys = sub
				}
			}
		}
		if fsys == nil {
			e := errAtRef("unknown built-in blueprint %q", ref.Ref)
			if s := didYouMean(ref.Ref, l.BuiltinNames()); s != "" {
				e.Suggestion = s
			} else if names := l.BuiltinNames(); len(names) > 0 {
				e.Suggestion = "built-in blueprints are " + joinNames(names)
			}
			return nil, diags
		}
		location = "built-in"
		file = "builtin/" + ref.Ref + "/" + ManifestFile
	case project.SourceLocal:
		dir = ref.Ref
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(l.ProjectDir, dir)
		}
		info, err := os.Stat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			errAtRef("blueprint folder %s does not exist", dir)
			return nil, diags
		case err != nil:
			errAtRef("cannot read blueprint folder: %v", err)
			return nil, diags
		case !info.IsDir():
			errAtRef("%s is not a folder", dir)
			return nil, diags
		}
		fsys = os.DirFS(dir)
		location = dir
		file = filepath.Join(dir, ManifestFile)
	case project.SourceGit:
		if l.Git == nil {
			errAtRef("blueprints from git are not available here")
			return nil, diags
		}
		var err error
		dir, commit, err = l.Git.Checkout(context.Background(), ref.Ref)
		if errors.Is(err, ErrNotFetched) {
			l.unfetched++
			diags.Warnf(ref.RefSpan.Pos, ref.RefSpan.End, "%v; annotations and requires are not checked until then", err)
			return nil, diags
		}
		if err != nil {
			errAtRef("blueprint %s: %v", ref.Ref, err)
			return nil, diags
		}
		source = ref.Ref
		fsys = os.DirFS(dir)
		location = ref.Ref + " (" + commit[:min(12, len(commit))] + ")"
		file = filepath.Join(dir, ManifestFile)
	}

	src, err := fs.ReadFile(fsys, ManifestFile)
	if dir != "" {
		if abs, aerr := filepath.Abs(filepath.Join(dir, ManifestFile)); aerr == nil && l.Overlay[abs] != nil {
			src, err = l.Overlay[abs], nil
		}
	}
	if err != nil {
		errAtRef("%s has no %s", location, ManifestFile).Suggestion = "every blueprint folder needs a manifest"
		return nil, diags
	}
	if l.sources == nil {
		l.sources = map[string][]byte{}
	}
	l.sources[file] = src

	m, mdiags := ParseManifest(file, src)
	diags = append(diags, mdiags...)
	if m == nil {
		errAtRef("blueprint %s has an invalid manifest", refString(ref))
		return nil, diags
	}
	if ref.Source == project.SourceBuiltin && m.Name != ref.Ref {
		diags.Errorf(diag.Pos{File: file, Line: 1, Col: 1}, diag.Pos{}, "built-in blueprint folder %s declares blueprint %s", ref.Ref, m.Name)
		return nil, diags
	}
	for _, h := range m.Hooks {
		if ref.Source == project.SourceBuiltin && l.HasBuiltinHook != nil {
			if !l.HasBuiltinHook(m.Name, h.Path) {
				diags.Errorf(h.Span.Pos, h.Span.End, "hook %s is not built into this esquadro", h.Path)
			}
			continue
		}
		if _, err := fs.Stat(fsys, h.Path); err != nil {
			diags.Errorf(h.Span.Pos, h.Span.End, "hook %s does not exist in the blueprint folder", h.Path)
		}
	}
	if diags.HasErrors() {
		return nil, diags
	}
	return &Blueprint{Manifest: m, FS: fsys, Ref: ref, Location: location, Dir: dir, Source: source, Commit: commit}, diags
}

func joinNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
