package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/project"
)

func runAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	noGen := fs.Bool("no-gen", false, "change app.esq only, without generating")
	trust := fs.Bool("trust", false, "let the hooks of blueprints from git run without asking")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro add [-no-gen] [-trust] <blueprint>[,<blueprint>...] [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Adds blueprints to the app block of the project in dir (default: the current")
		fmt.Fprintln(stderr, "directory), with the blueprints they require, and generates. A blueprint is")
		fmt.Fprintln(stderr, "a built-in name (esquadro blueprint list), a local folder (./blueprints/x)")
		fmt.Fprintln(stderr, "or a git source (github.com/acme/x@v1). Its options are written commented")
		fmt.Fprintln(stderr, "out, with their defaults, to change when you need to.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	refs, dir, ok := parseManageArgs("add", fs, args, stderr)
	if !ok {
		return 2
	}
	// The project may have errors the blueprints to add fix (a @jobs
	// annotation, before the jobs blueprint): it is checked once changed.
	sources, err := readProject(dir)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro add: %v\n", err)
		return 1
	}
	l, _ := loadAll("add", dir, sources, io.Discard, false)
	if l == nil {
		return 1
	}
	app, err := findApp(l.sources)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro add: %v\n", err)
		return 1
	}
	present := map[string]bool{}
	for _, r := range app.decl.Blueprints {
		if r.Ref.Kind == dsl.IdentValue {
			present[r.Ref.Text] = true
		} else if b, _ := l.loader.Load(refOf(r.Ref.Text)); b != nil {
			present[b.Manifest.Name] = true
		}
	}

	// The blueprints to add, each after those it requires.
	has := func(name string) bool { return present[name] }
	var added []*blueprint.Blueprint
	adding := map[string]bool{}
	var visit func(ref *project.BlueprintRef, requiredBy string) bool
	visit = func(ref *project.BlueprintRef, requiredBy string) bool {
		b, diags := l.loader.Load(ref)
		if diags.HasErrors() || b == nil {
			if requiredBy != "" {
				fmt.Fprintf(stderr, "esquadro add: %s requires %s, which is not built in: add it first, from where it comes\n", requiredBy, ref.Ref)
			} else {
				fmt.Fprintf(stderr, "esquadro add: there is no blueprint %s\n", ref.Ref)
				for _, d := range diags {
					fmt.Fprintf(stderr, "  %s\n", d.Message)
				}
			}
			return false
		}
		name := b.Manifest.Name
		if has(name) || adding[name] {
			if requiredBy == "" {
				fmt.Fprintf(stdout, "%s is already in the app\n", name)
			}
			return true
		}
		adding[name] = true
		for _, dep := range b.Manifest.Requires {
			if !visit(&project.BlueprintRef{Ref: dep.Name, Source: project.SourceBuiltin}, name) {
				return false
			}
		}
		added = append(added, b)
		return true
	}
	for _, r := range refs {
		if !visit(refOf(r), "") {
			return 1
		}
	}
	if len(added) == 0 {
		return 0
	}

	var block strings.Builder
	for _, b := range added {
		block.WriteString(blueprintBlock(b))
	}
	src := app.src
	at := app.insertAt()
	changed := slices.Concat(src[:at], []byte(block.String()), src[at:])
	sources = withSource(l.sources, app.name, changed)
	if loadSources("add", dir, sources, stderr, false) == nil {
		fmt.Fprintln(stderr, "esquadro add: app.esq was not changed")
		return 1
	}
	if err := os.WriteFile(app.name, changed, 0o644); err != nil {
		fmt.Fprintf(stderr, "esquadro add: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, addedMessage(added, refs))
	if *noGen {
		return 0
	}
	genArgs := []string{dir}
	if *trust {
		genArgs = []string{"-trust", dir}
	}
	return runGen(genArgs, stdout, stderr)
}

// parseManageArgs reads the flags and the "<blueprint>[,...] [dir]" of add
// and remove; flags may follow the blueprints.
func parseManageArgs(cmd string, fs *flag.FlagSet, args []string, stderr io.Writer) (refs []string, dir string, ok bool) {
	var positional []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			return nil, "", false
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(positional) == 0 || len(positional) > 2 {
		fmt.Fprintf(stderr, "esquadro %s: expected the blueprints and, optionally, the project's directory\n", cmd)
		fs.Usage()
		return nil, "", false
	}
	for r := range strings.SplitSeq(positional[0], ",") {
		if r = strings.TrimSpace(r); r != "" {
			refs = append(refs, r)
		}
	}
	dir = "."
	if len(positional) == 2 {
		dir = positional[1]
	}
	return refs, dir, len(refs) > 0
}

// refOf is a blueprint as given on the command line.
func refOf(r string) *project.BlueprintRef {
	ref := &project.BlueprintRef{Ref: r, Source: project.SourceBuiltin}
	switch {
	case strings.HasPrefix(r, "./") || strings.HasPrefix(r, "../") || strings.HasPrefix(r, "/"):
		ref.Source = project.SourceLocal
	case strings.Contains(r, "/"):
		ref.Source = project.SourceGit
	}
	return ref
}

// blueprintBlock is a blueprint's line in the app block, with its options
// commented out at their defaults; an option without a default is written
// with an example value, to fill in.
func blueprintBlock(b *blueprint.Blueprint) string {
	ref := b.Ref.Ref
	if b.Ref.Source != project.SourceBuiltin {
		ref = strconv.Quote(ref)
	}
	if len(b.Manifest.Options) == 0 {
		return fmt.Sprintf("  blueprint %s\n", ref)
	}
	var s strings.Builder
	fmt.Fprintf(&s, "  blueprint %s {\n", ref)
	for _, o := range b.Manifest.Options {
		if o.Default == nil {
			fmt.Fprintf(&s, "    %s %s\n", o.Name, blueprint.Example(o.Type))
			continue
		}
		fmt.Fprintf(&s, "    // %s %s\n", o.Name, literal(o.Default))
	}
	s.WriteString("  }\n")
	return s.String()
}

// literal writes an option's value as in a .esq file.
func literal(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case []string:
		q := make([]string, len(v))
		for i, s := range v {
			q[i] = strconv.Quote(s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	case []any:
		q := make([]string, len(v))
		for i, s := range v {
			q[i] = literal(s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	return fmt.Sprint(v)
}

// addedMessage says what add added: the blueprints asked for, and those they
// required.
func addedMessage(added []*blueprint.Blueprint, asked []string) string {
	var mine, required []string
	for _, b := range added {
		if slices.Contains(asked, b.Ref.Ref) {
			mine = append(mine, b.Manifest.Name)
		} else {
			required = append(required, b.Manifest.Name)
		}
	}
	msg := "added " + strings.Join(mine, ", ")
	if len(mine) == 0 {
		msg = "added"
	}
	if len(required) > 0 {
		verb := "it requires"
		if len(mine) > 1 {
			verb = "they require"
		}
		msg += fmt.Sprintf(" (and %s, which %s)", strings.Join(required, ", "), verb)
	}
	return msg + " to app.esq"
}

// appSource is the .esq file with the app block, parsed.
type appSource struct {
	name string
	src  []byte
	decl *dsl.AppDecl
}

func findApp(sources []project.Source) (*appSource, error) {
	for _, s := range sources {
		f, _ := dsl.Parse(s.Name, s.Src)
		if f == nil {
			continue
		}
		for _, d := range f.Decls {
			if app, ok := d.(*dsl.AppDecl); ok {
				return &appSource{name: s.Name, src: s.Src, decl: app}, nil
			}
		}
	}
	return nil, fmt.Errorf("no file of the project has the app block")
}

// insertAt is where new blueprints go: after the last one, but before git,
// which stays last; or after the module line.
func (a *appSource) insertAt() int {
	after := -1
	for _, item := range a.decl.Items {
		if r, ok := item.(*dsl.BlueprintRef); ok && r.Ref.Text == "git" {
			break
		}
		switch item.(type) {
		case *dsl.BlueprintRef, *dsl.ModuleItem:
			after = item.SourceSpan().End.Offset
		}
	}
	if after < 0 {
		after = a.decl.Name.End.Offset
	}
	// The start of the next line.
	if i := bytes.IndexByte(a.src[after:], '\n'); i >= 0 {
		return after + i + 1
	}
	return len(a.src)
}

// withSource is sources with one file's content replaced.
func withSource(sources []project.Source, name string, src []byte) []project.Source {
	out := slices.Clone(sources)
	for i := range out {
		if out[i].Name == name {
			out[i].Src = src
		}
	}
	return out
}
