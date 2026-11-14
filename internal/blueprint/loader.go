package blueprint

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/model"
)

// Blueprint is a loaded blueprint: its manifest and its files.
type Blueprint struct {
	Manifest *Manifest
	// FS is rooted at the blueprint's folder.
	FS  fs.FS
	Ref *model.BlueprintRef
	// Location is where the blueprint came from, for messages:
	// "built-in", or the folder path.
	Location string
}

// Loader finds blueprints.
type Loader struct {
	// Builtin holds the built-in blueprints, one folder per blueprint.
	Builtin fs.FS
	// ProjectDir is the folder local blueprint paths are relative to.
	ProjectDir string
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
func (l *Loader) Load(ref *model.BlueprintRef) (*Blueprint, diag.List) {
	var diags diag.List
	errAtRef := func(format string, args ...any) *diag.Diagnostic {
		return diags.Errorf(ref.RefSpan.Pos, ref.RefSpan.End, format, args...)
	}

	var (
		fsys     fs.FS
		location string
		file     string
	)
	switch ref.Source {
	case model.SourceBuiltin:
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
	case model.SourceLocal:
		dir := ref.Ref
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
	case model.SourceGit:
		errAtRef("blueprints from git are not supported yet").
			Suggestion = "clone it into a folder and use the local path, e.g. blueprint \"./blueprints/name\""
		return nil, diags
	}

	src, err := fs.ReadFile(fsys, ManifestFile)
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
	if ref.Source == model.SourceBuiltin && m.Name != ref.Ref {
		diags.Errorf(diag.Pos{File: file, Line: 1, Col: 1}, diag.Pos{}, "built-in blueprint folder %s declares blueprint %s", ref.Ref, m.Name)
		return nil, diags
	}
	for _, h := range m.Hooks {
		if _, err := fs.Stat(fsys, h.Path); err != nil {
			diags.Errorf(h.Span.Pos, h.Span.End, "hook %s does not exist in the blueprint folder", h.Path)
		}
	}
	if diags.HasErrors() {
		return nil, diags
	}
	return &Blueprint{Manifest: m, FS: fsys, Ref: ref, Location: location}, diags
}

func joinNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
