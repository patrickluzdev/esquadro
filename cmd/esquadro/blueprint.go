package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/hook"
	"github.com/patrickluzdev/esquadro/internal/project"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// blueprintCommands are the subcommands of esquadro blueprint.
func blueprintCommands() []command {
	return []command{
		{"list", "list the blueprints you can use, and those the project uses", runBlueprintList, false},
		{"info", "show what a blueprint does: its options, slots and files", runBlueprintInfo, false},
		{"update", "move blueprints from git to the latest commit of their ref", runBlueprintUpdate, false},
		{"new", "start a blueprint of your own in the project", runBlueprintNew, false},
		{"test", "generate a blueprint's test project and compare it with the expected output", runBlueprintTest, false},
	}
}

func runBlueprint(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		blueprintUsage(stdout)
		return 0
	}
	for _, c := range blueprintCommands() {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "esquadro blueprint: unknown command %q\n\n", args[0])
	blueprintUsage(stderr)
	return 2
}

func blueprintUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: esquadro blueprint <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range blueprintCommands() {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.short)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "To add a blueprint to the project: esquadro add; to remove one: esquadro remove.")
}

// oneDir reads the flags and an optional directory.
func oneDir(cmd string, fs *flag.FlagSet, args []string, stderr io.Writer) (string, bool) {
	if err := fs.Parse(args); err != nil {
		return "", false
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "esquadro %s: expected one directory, got %d arguments\n", cmd, fs.NArg())
		fs.Usage()
		return "", false
	}
	if fs.NArg() == 1 {
		return fs.Arg(0), true
	}
	return ".", true
}

// isProject reports whether dir has an esquadro/ folder.
func isProject(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, workspace.Dir))
	return err == nil && info.IsDir()
}

// builtinLoader loads built-in blueprints, outside a project.
func builtinLoader() *blueprint.Loader {
	return &blueprint.Loader{
		Builtin:  blueprints.FS,
		Browsing: true,
		HasBuiltinHook: func(bp, path string) bool {
			return builtinHooks[hook.Key(bp, path)] != nil
		},
	}
}

// summary is the first sentence of a blueprint's documentation.
func summary(doc []string) string {
	s, _, _ := strings.Cut(strings.Join(doc, " "), ". ")
	return strings.TrimSuffix(s, ".")
}

func runBlueprintList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("blueprint list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro blueprint list [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Lists the built-in blueprints, by what they are for. In a project (dir,")
		fmt.Fprintln(stderr, "default: the current directory), marks those the app uses with *, and lists")
		fmt.Fprintln(stderr, "its blueprints from folders and git too.")
	}
	dir, ok := oneDir("blueprint list", fs, args, stderr)
	if !ok {
		return 2
	}
	var l *loadedProject
	if isProject(dir) {
		if l = loadProject("blueprint list", dir, io.Discard, false); l == nil {
			fmt.Fprintln(stderr, "esquadro blueprint list: the project has errors (see esquadro check); listing the built-in blueprints only")
		}
	}
	used := func(name string) bool { return l != nil && l.set.Get(name) != nil }

	loader := builtinLoader()
	order, groups := blueprintOrder, blueprintGroups
	if l != nil && l.project.Kind == project.KindPlatform {
		loader.Kind = project.KindPlatform
		order, groups = platformBlueprintOrder, platformGroups
	}
	mark := "  "
	if l != nil {
		fmt.Fprintln(stdout, "Built-in blueprints (* the app uses it):")
	} else {
		fmt.Fprintln(stdout, "Built-in blueprints:")
	}
	group := ""
	for _, name := range order {
		b, diags := loader.Load(&project.BlueprintRef{Ref: name, Source: project.SourceBuiltin})
		if b == nil || diags.HasErrors() {
			fmt.Fprintf(stderr, "esquadro blueprint list: %s: %v\n", name, diags)
			return 1
		}
		if g := groups[name]; g != group {
			group = g
			fmt.Fprintf(stdout, "\n  %s\n", groupTitles[g])
		}
		mark = "  "
		if used(name) {
			mark = "* "
		}
		fmt.Fprintf(stdout, "  %s%-17s %-6s %s\n", mark, name, b.Manifest.Version, clip(summary(b.Manifest.Doc), 56))
	}
	if l != nil {
		var own []*blueprint.Blueprint
		for _, b := range l.set.Blueprints {
			if b.Ref.Source != project.SourceBuiltin {
				own = append(own, b)
			}
		}
		if len(own) > 0 {
			fmt.Fprintln(stdout, "\nThe project's own:")
			for _, b := range own {
				fmt.Fprintf(stdout, "  * %-17s %-6s %s\n      %s\n", b.Manifest.Name, b.Manifest.Version, clip(summary(b.Manifest.Doc), 56), b.Location)
			}
		}
	}
	fmt.Fprintln(stdout, "\nMore on one: esquadro blueprint info <name>. To use one: esquadro add <name>.")
	return 0
}

func runBlueprintInfo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("blueprint info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro blueprint info <blueprint> [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Shows what a blueprint does, the blueprints it works with, its options,")
		fmt.Fprintln(stderr, "annotations and slots, and the files it generates. The blueprint is a")
		fmt.Fprintln(stderr, "built-in name, one of the project's in dir (default: the current directory),")
		fmt.Fprintln(stderr, "a local folder or a git source.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fmt.Fprintln(stderr, "esquadro blueprint info: expected a blueprint and, optionally, the project's directory")
		fs.Usage()
		return 2
	}
	name, dir := fs.Arg(0), "."
	if fs.NArg() == 2 {
		dir = fs.Arg(1)
	}
	var b *blueprint.Blueprint
	if isProject(dir) {
		if l := loadProject("blueprint info", dir, io.Discard, false); l != nil {
			b = l.set.Get(name)
			if b == nil {
				var diags []string
				if b, diags = loadRef(l.loader, name); b == nil {
					fmt.Fprintf(stderr, "esquadro blueprint info: there is no blueprint %s\n%s", name, strings.Join(diags, ""))
					return 1
				}
			}
		}
	}
	if b == nil {
		var diags []string
		loader := builtinLoader()
		loader.ProjectDir = filepath.Join(dir, workspace.Dir)
		if cache, err := blueprint.DefaultCacheDir(); err == nil {
			loader.Git = &blueprint.Git{CacheDir: cache}
		}
		if b, diags = loadRef(loader, name); b == nil {
			fmt.Fprintf(stderr, "esquadro blueprint info: there is no blueprint %s\n%s", name, strings.Join(diags, ""))
			return 1
		}
	}
	fmt.Fprint(stdout, blueprintInfo(b))
	return 0
}

// loadRef loads a blueprint given as on the command line.
func loadRef(l *blueprint.Loader, ref string) (*blueprint.Blueprint, []string) {
	b, diags := l.Load(refOf(ref))
	if b == nil || diags.HasErrors() {
		var msgs []string
		for _, d := range diags {
			msgs = append(msgs, "  "+d.Message+"\n")
		}
		return nil, msgs
	}
	return b, nil
}

// blueprintInfo describes a blueprint for the terminal.
func blueprintInfo(b *blueprint.Blueprint) string {
	m := b.Manifest
	var s strings.Builder
	fmt.Fprintf(&s, "%s %s (%s)\n\n", m.Name, m.Version, b.Location)
	if len(m.Doc) > 0 {
		s.WriteString(wrap(paragraphs(m.Doc), "  "))
		s.WriteString("\n\n")
	}
	names := func(deps []blueprint.Dep) string {
		var n []string
		for _, d := range deps {
			n = append(n, d.Name)
		}
		return strings.Join(n, ", ")
	}
	if len(m.Requires) > 0 {
		fmt.Fprintf(&s, "Requires:   %s\n", names(m.Requires))
	}
	if len(m.Optional) > 0 {
		fmt.Fprintf(&s, "Works with: %s\n", names(m.Optional))
	}
	if len(m.Requires)+len(m.Optional) > 0 {
		s.WriteString("\n")
	}
	if len(m.Options) > 0 {
		s.WriteString("Options:\n")
		for _, o := range m.Options {
			def := "required"
			if !o.Required {
				def = "default " + valueText(o.Default)
			}
			fmt.Fprintf(&s, "  %s %s, %s\n", o.Name, o.Type, def)
			if len(o.Doc) > 0 {
				fmt.Fprintf(&s, "%s\n", wrap(strings.Join(o.Doc, " "), "      "))
			}
		}
		s.WriteString("\n")
	}
	if len(m.Env) > 0 {
		s.WriteString("Environment variables:\n")
		for _, v := range m.Env {
			fmt.Fprintf(&s, "  %s %s; %s\n", envDisplayName(v), envKind(v), strings.ReplaceAll(envDefaults(v), "`", ""))
			if len(v.Doc) > 0 {
				fmt.Fprintf(&s, "%s\n", wrap(strings.Join(v.Doc, " "), "      "))
			}
		}
		s.WriteString("\n")
	}
	if len(m.Annotations) > 0 {
		fmt.Fprintf(&s, "Annotations, written @%s(key value):\n", m.Name)
		for _, t := range []blueprint.Target{blueprint.TargetApp, blueprint.TargetEntity, blueprint.TargetField, blueprint.TargetRelation, blueprint.TargetJob} {
			spec := m.Annotations[t]
			if spec == nil {
				continue
			}
			for _, k := range spec.Keys {
				fmt.Fprintf(&s, "  on a %s: %s %s\n", t, k.Name, k.Type)
				if len(k.Doc) > 0 {
					fmt.Fprintf(&s, "%s\n", wrap(strings.Join(k.Doc, " "), "      "))
				}
			}
		}
		s.WriteString("\n")
	}
	if len(m.Slots) > 0 {
		s.WriteString("Slots, where other blueprints add code:\n")
		for _, sl := range m.Slots {
			fmt.Fprintf(&s, "  %s\n", sl.Name)
			if len(sl.Doc) > 0 {
				fmt.Fprintf(&s, "%s\n", wrap(strings.Join(sl.Doc, " "), "      "))
			}
		}
		s.WriteString("\n")
	}
	if files := templateFiles(b.FS); len(files) > 0 {
		s.WriteString("Files it generates:\n")
		for _, f := range files {
			fmt.Fprintf(&s, "  %s\n", f)
		}
		s.WriteString("\n")
	}
	if contribs := slotFiles(b.FS); len(contribs) > 0 {
		fmt.Fprintf(&s, "%s\n\n", wrap("Adds code to the slots "+strings.Join(contribs, ", ")+".", ""))
	}
	return s.String()
}

// templateFiles are the paths of a blueprint's templates, as written.
func templateFiles(fsys fs.FS) []string {
	var files []string
	fs.WalkDir(fsys, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		f := strings.TrimSuffix(strings.TrimPrefix(p, "templates/"), ".tmpl")
		f = templateVar.ReplaceAllStringFunc(f, func(v string) string {
			return "<" + placeholder(templateVar.FindStringSubmatch(v)[1]) + ">"
		})
		if once, ok := strings.CutSuffix(f, ".once"); ok {
			f = once + "  (yours: written once, then left alone)"
		}
		files = append(files, f)
		return nil
	})
	return files
}

// slotFiles are the slots a blueprint contributes to.
func slotFiles(fsys fs.FS) []string {
	entries, err := fs.ReadDir(fsys, "slots")
	if err != nil {
		return nil
	}
	var slots []string
	for _, e := range entries {
		slots = append(slots, strings.TrimSuffix(e.Name(), ".tmpl"))
	}
	return slots
}

var templateVar = regexp.MustCompile(`\{\{-?\s*\.([A-Za-z.]+)\s*-?\}\}`)

// placeholder names what a template's value in a path stands for:
// .App.Kebab is app, .Entity.Module.Snake is module.
func placeholder(v string) string {
	parts := strings.Split(v, ".")
	switch {
	case parts[0] == "Options" && len(parts) > 1:
		return parts[1]
	case slices.Contains(parts, "Module"):
		return "module"
	}
	return strings.ToLower(parts[0])
}

// clip shortens text to n characters: to what comes before a colon or a
// parenthesis, when that says enough, or at a word.
func clip(text string, n int) string {
	if len(text) <= n {
		return text
	}
	if i := strings.IndexAny(text, ":("); i >= 12 && i <= n {
		return strings.TrimSpace(text[:i])
	}
	cut := strings.LastIndexByte(text[:n], ' ')
	if cut < n/2 {
		cut = n
	}
	return strings.TrimRight(text[:cut], " ,:;") + "…"
}

// wrap breaks text into lines of at most 78 characters, each with prefix.
func wrap(text, prefix string) string {
	var out []string
	for para := range strings.SplitSeq(text, "\n") {
		line := ""
		for w := range strings.FieldsSeq(para) {
			if line != "" && len(prefix)+len(line)+1+len(w) > 78 {
				out = append(out, prefix+line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += w
		}
		if line != "" {
			out = append(out, prefix+line)
		} else {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

func runBlueprintUpdate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("blueprint update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	trust := fs.Bool("trust", false, "let the hooks of the new commits run without asking")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro blueprint update [-trust] [<blueprint>[,...]] [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Moves the project's blueprints from git (all, or those named) to the commit")
		fmt.Fprintln(stderr, "their ref points to now, and generates. The lockfile keeps each one at its")
		fmt.Fprintln(stderr, "commit until then. Built-in blueprints come with esquadro: update esquadro.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var names []string
	dir := "."
	switch fs.NArg() {
	case 0:
	case 1:
		if isProject(fs.Arg(0)) {
			dir = fs.Arg(0)
		} else {
			names = strings.Split(fs.Arg(0), ",")
		}
	case 2:
		names, dir = strings.Split(fs.Arg(0), ","), fs.Arg(1)
	default:
		fs.Usage()
		return 2
	}
	lock, err := workspace.ReadLock(dir)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro blueprint update: %v\n", err)
		return 1
	}
	var fromGit []string
	for _, b := range lock.Blueprints {
		if b.Source != "" {
			fromGit = append(fromGit, b.Name)
		}
	}
	if len(fromGit) == 0 {
		fmt.Fprintln(stdout, "The project uses no blueprints from git: the built-in ones update with esquadro itself.")
		return 0
	}
	for _, n := range names {
		if !slices.Contains(fromGit, n) {
			fmt.Fprintf(stderr, "esquadro blueprint update: %s is not a blueprint from git of the project; those are %s\n", n, strings.Join(fromGit, ", "))
			return 1
		}
	}
	before := map[string]string{}
	unpinned := *lock
	unpinned.Blueprints = slices.Clone(lock.Blueprints)
	for i, b := range unpinned.Blueprints {
		if b.Source != "" && (len(names) == 0 || slices.Contains(names, b.Name)) {
			before[b.Name] = b.Commit
			unpinned.Blueprints[i].Commit = ""
			unpinned.Blueprints[i].Trusted = false
		}
	}
	if err := workspace.WriteLock(dir, &unpinned); err != nil {
		fmt.Fprintf(stderr, "esquadro blueprint update: %v\n", err)
		return 1
	}
	genArgs := []string{dir}
	if *trust {
		genArgs = []string{"-trust", dir}
	}
	if code := runGen(genArgs, stdout, stderr); code != 0 {
		// Back to the commits the project had.
		if err := workspace.WriteLock(dir, lock); err != nil {
			fmt.Fprintf(stderr, "esquadro blueprint update: %v\n", err)
		}
		return code
	}
	after, err := workspace.ReadLock(dir)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro blueprint update: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout)
	for _, b := range after.Blueprints {
		old, ok := before[b.Name]
		if !ok {
			continue
		}
		if old == b.Commit {
			fmt.Fprintf(stdout, "%s: already at the latest commit, %s\n", b.Name, short(b.Commit))
		} else {
			fmt.Fprintf(stdout, "%s: %s -> %s\n", b.Name, short(old), short(b.Commit))
		}
	}
	return 0
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
