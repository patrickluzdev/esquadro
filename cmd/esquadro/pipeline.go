package main

import (
	"fmt"
	"io"
	"path/filepath"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/blueprints/app"
	"github.com/patrickluzdev/esquadro/blueprints/git"
	"github.com/patrickluzdev/esquadro/blueprints/i18n"
	"github.com/patrickluzdev/esquadro/blueprints/migrations"
	"github.com/patrickluzdev/esquadro/blueprints/rest"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/hook"
	"github.com/patrickluzdev/esquadro/internal/project"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// builtinHooks are the hooks of the built-in blueprints, by hook.Key.
var builtinHooks = map[string]hook.Func{
	hook.Key("app", "go-mod"):       app.GoMod,
	hook.Key("migrations", "write"): migrations.Write,
	hook.Key("store", "sqlc"):       store.SQLC,
	hook.Key("rest", "ogen"):        rest.Ogen,
	hook.Key("git", "init"):         git.Init,
}

// builtinFuncs are the template helpers of the built-in blueprints, by name.
var builtinFuncs = map[string]template.FuncMap{
	"store": store.TemplateFuncs(),
	"rest":  rest.TemplateFuncs(),
	"i18n":  i18n.TemplateFuncs(),
}

// loadedProject is a project read from its folder, with its blueprints
// resolved.
type loadedProject struct {
	dir     string
	sources []project.Source
	loader  *blueprint.Loader
	project *project.Project
	set     *blueprint.Set
}

// source returns a project file or a blueprint manifest by name, for
// rendering diagnostics; extra looks up other files, such as templates.
func (l *loadedProject) source(extra func(string) []byte) diag.SourceFunc {
	return func(name string) []byte {
		for _, s := range l.sources {
			if s.Name == name {
				return s.Src
			}
		}
		if src := l.loader.Source(name); src != nil {
			return src
		}
		if extra != nil {
			return extra(name)
		}
		return nil
	}
}

// loadProject reads and validates the project in dir and resolves its
// blueprints, printing every diagnostic to stderr. It returns nil when there
// are errors.
func loadProject(cmd, dir string, stderr io.Writer) *loadedProject {
	sources, err := readProject(dir)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro %s: %v\n", cmd, err)
		return nil
	}
	l := &loadedProject{
		dir:     dir,
		sources: sources,
		loader: &blueprint.Loader{
			Builtin:    blueprints.FS,
			ProjectDir: filepath.Join(dir, workspace.Dir),
			HasBuiltinHook: func(bp, path string) bool {
				return builtinHooks[hook.Key(bp, path)] != nil
			},
		},
	}
	p, diags := project.Load(sources...)
	if !diags.HasErrors() {
		set, more := blueprint.Resolve(p, l.loader)
		diags = append(diags, more...)
		diags.Sort()
		l.project, l.set = p, set
	}
	if len(diags) > 0 {
		diag.Render(stderr, diags, l.source(nil))
	}
	if diags.HasErrors() {
		return nil
	}
	return l
}
