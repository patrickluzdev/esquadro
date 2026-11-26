package main

import (
	"fmt"
	"io"
	"path/filepath"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/blueprints/app"
	"github.com/patrickluzdev/esquadro/blueprints/audit"
	"github.com/patrickluzdev/esquadro/blueprints/branching"
	clientts "github.com/patrickluzdev/esquadro/blueprints/client_ts"
	"github.com/patrickluzdev/esquadro/blueprints/domain"
	"github.com/patrickluzdev/esquadro/blueprints/env"
	"github.com/patrickluzdev/esquadro/blueprints/git"
	"github.com/patrickluzdev/esquadro/blueprints/i18n"
	"github.com/patrickluzdev/esquadro/blueprints/integration"
	"github.com/patrickluzdev/esquadro/blueprints/jobs"
	mailbp "github.com/patrickluzdev/esquadro/blueprints/mail"
	"github.com/patrickluzdev/esquadro/blueprints/migrations"
	"github.com/patrickluzdev/esquadro/blueprints/notifications"
	"github.com/patrickluzdev/esquadro/blueprints/platform/apps"
	ratelimit "github.com/patrickluzdev/esquadro/blueprints/rate_limit"
	"github.com/patrickluzdev/esquadro/blueprints/rest"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/seed"
	"github.com/patrickluzdev/esquadro/blueprints/spa"
	"github.com/patrickluzdev/esquadro/blueprints/ssr"
	"github.com/patrickluzdev/esquadro/blueprints/storage"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/blueprints/ui"
	bpworkspace "github.com/patrickluzdev/esquadro/blueprints/workspace"
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
	hook.Key("ssr", "templ"):        ssr.Templ,
	hook.Key("git", "init"):         git.Init,
	hook.Key("branching", "setup"):  branching.Setup,
	hook.Key("env", "files"):        env.Files,
	hook.Key("workspace", "ignore"): bpworkspace.Ignore,
	hook.Key("storage", "ignore"):   storage.Ignore,
}

func init() {
	blueprint.Checks["security"] = security.Check
	blueprint.Checks["store"] = store.Check
	blueprint.Checks["rest"] = rest.Check
	blueprint.Checks["client_ts"] = clientts.Check
	blueprint.Checks["platform/apps"] = apps.Check
	blueprint.Checks["workspace"] = bpworkspace.Check
	blueprint.Checks["spa"] = spa.Check
	blueprint.Checks["ui"] = ui.Check
	blueprint.Checks["jobs"] = jobs.Check
	blueprint.Checks["ssr"] = ssr.Check
	blueprint.Checks["seed"] = seed.Check
	blueprint.Checks["audit"] = audit.Check
	blueprint.Checks["notifications"] = notifications.Check
	blueprint.Checks["storage"] = storage.Check
	blueprint.Checks["integration"] = integration.Check
	blueprint.Checks["rate_limit"] = ratelimit.Check
	blueprint.Checks["mail"] = mailbp.Check
	blueprint.Checks["git"] = git.Check
}

// builtinFuncs are the template helpers of the built-in blueprints, by name.
var builtinFuncs = map[string]template.FuncMap{
	"app":           app.TemplateFuncs(),
	"domain":        domain.TemplateFuncs(),
	"store":         store.TemplateFuncs(),
	"rest":          rest.TemplateFuncs(),
	"client_ts":     clientts.TemplateFuncs(),
	"i18n":          i18n.TemplateFuncs(),
	"jobs":          jobs.TemplateFuncs(),
	"security":      security.TemplateFuncs(),
	"spa":           spa.TemplateFuncs(),
	"ssr":           ssr.TemplateFuncs(),
	"seed":          seed.TemplateFuncs(),
	"audit":         audit.TemplateFuncs(),
	"notifications": notifications.TemplateFuncs(),
	"storage":       storage.TemplateFuncs(),
	"integration":   integration.TemplateFuncs(),
	"mail":          mailbp.TemplateFuncs(),
}

// loadedProject is a project read from its folder, with its blueprints
// resolved.
type loadedProject struct {
	dir     string
	sources []project.Source
	loader  *blueprint.Loader
	project *project.Project
	set     *blueprint.Set
	// lock is the lockfile of the last generation.
	lock *workspace.Lock
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
func loadProject(cmd, dir string, stderr io.Writer, update bool) *loadedProject {
	sources, err := readProject(dir)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro %s: %v\n", cmd, err)
		return nil
	}
	return loadSources(cmd, dir, sources, stderr, update)
}

// loadSources is loadProject with the project's files given, as read or as
// they would be after a change.
func loadSources(cmd, dir string, sources []project.Source, stderr io.Writer, update bool) *loadedProject {
	l, ok := loadAll(cmd, dir, sources, stderr, update)
	if !ok {
		return nil
	}
	return l
}

// loadAll is loadSources that also returns, when the project has errors,
// what could be loaded: its blueprint loader always, its project and
// blueprints when they got that far. It returns nil only when the lockfile
// or the cache cannot be read.
func loadAll(cmd, dir string, sources []project.Source, stderr io.Writer, update bool) (*loadedProject, bool) {
	lock, err := workspace.ReadLock(dir)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro %s: %v\n", cmd, err)
		return nil, false
	}
	cache, err := blueprint.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(stderr, "esquadro %s: %v\n", cmd, err)
		return nil, false
	}
	pinned := map[string]string{}
	for _, b := range lock.Blueprints {
		if b.Source != "" && b.Commit != "" {
			pinned[b.Source] = b.Commit
		}
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
			Git: &blueprint.Git{CacheDir: cache, Pinned: pinned, Update: update},
		},
		lock: lock,
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
	return l, !diags.HasErrors()
}
