package blueprint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
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
	// Key names the blueprint's Go code built into esquadro: its hooks,
	// template helpers and checks. It is the manifest's name, prefixed with
	// platform/ for a built-in blueprint of the platform catalog, whose
	// names may repeat the apps' (postgres).
	Key string
}

// PlatformDir is the folder of Builtin that holds the blueprints made for
// platforms only; the others are at its root.
const PlatformDir = "platform"

// Loader finds blueprints.
type Loader struct {
	// Builtin holds the built-in blueprints, one folder per blueprint; those
	// only for platforms are in PlatformDir.
	Builtin fs.FS
	// Kind is the kind of project blueprints are loaded for: a project only
	// uses blueprints made for its kind.
	Kind project.Kind
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
// Retired are built-in blueprints that are no more, with what replaces
// them.
var Retired = map[string]string{
	"cors":             "blueprint http answers CORS; remove it, and list the origins in blueprint http, with its cors_origins option",
	"security_headers": "blueprint security sends the security headers; replace it with blueprint security, with its hsts option",
}

func (l *Loader) Source(file string) []byte { return l.sources[file] }

// kind is the kind of project blueprints are loaded for, app by default.
func (l *Loader) kind() project.Kind {
	if l.Kind == "" {
		return project.KindApp
	}
	return l.Kind
}

// BuiltinNames lists the built-in blueprints made for the loader's kind of
// project.
func (l *Loader) BuiltinNames() []string {
	if l.Builtin == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, dir := range []string{PlatformDir, "."} {
		entries, err := fs.ReadDir(l.Builtin, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || seen[e.Name()] {
				continue
			}
			src, err := fs.ReadFile(l.Builtin, path.Join(dir, e.Name(), ManifestFile))
			if err != nil {
				continue
			}
			if m, _ := ParseManifest(e.Name(), src); m != nil && slices.Contains(m.For, l.kind()) {
				seen[e.Name()] = true
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	return names
}

// builtin finds a built-in blueprint for the loader's kind of project: in
// PlatformDir first for a platform, then at the root. It answers its
// folder's path.
func (l *Loader) builtin(name string) (fs.FS, string) {
	if l.Builtin == nil || name == PlatformDir {
		return nil, ""
	}
	dirs := []string{name}
	if l.kind() == project.KindPlatform {
		dirs = []string{path.Join(PlatformDir, name), name}
	}
	for _, dir := range dirs {
		if sub, err := fs.Sub(l.Builtin, dir); err == nil {
			if _, err := fs.Stat(sub, ManifestFile); err == nil {
				return sub, dir
			}
		}
	}
	return nil, ""
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
		key            string
	)
	switch ref.Source {
	case project.SourceBuiltin:
		var at string
		fsys, at = l.builtin(ref.Ref)
		key = at
		if fsys == nil {
			if msg, ok := Retired[ref.Ref]; ok {
				errAtRef("blueprint %s is no more: %s", ref.Ref, msg)
				return nil, diags
			}
			e := errAtRef("unknown built-in blueprint %q", ref.Ref)
			if s := didYouMean(ref.Ref, l.BuiltinNames()); s != "" {
				e.Suggestion = s
			} else if names := l.BuiltinNames(); len(names) > 0 {
				e.Suggestion = "built-in blueprints are " + joinNames(names)
			}
			return nil, diags
		}
		location = "built-in"
		file = "builtin/" + at + "/" + ManifestFile
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
	if !slices.Contains(m.For, l.kind()) {
		other := "app projects"
		if m.For[0] == project.KindPlatform {
			other = "platforms"
		}
		errAtRef("blueprint %s is made for %s, not for %s", m.Name, other, map[project.Kind]string{project.KindApp: "an app", project.KindPlatform: "a platform"}[l.kind()]).
			Suggestion = "see the blueprints this project can use with esquadro blueprint list"
		return nil, diags
	}
	if key == "" {
		key = m.Name
	}
	for _, h := range m.Hooks {
		if ref.Source == project.SourceBuiltin && l.HasBuiltinHook != nil {
			if !l.HasBuiltinHook(key, h.Path) {
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
	return &Blueprint{Manifest: m, FS: fsys, Ref: ref, Location: location, Dir: dir, Source: source, Commit: commit, Key: key}, diags
}

func joinNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
