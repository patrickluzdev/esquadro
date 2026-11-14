package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// manifestName is the blueprint manifest file, which is not a project file.
const manifestName = "blueprint.esq"

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro check [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Checks the project in dir (default: the current directory) for syntax and")
		fmt.Fprintln(stderr, "semantic errors without generating anything. Every .esq file in dir except")
		fmt.Fprintln(stderr, "blueprint.esq is part of the project; subdirectories are not read.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "esquadro check: expected one directory, got %d arguments\n", fs.NArg())
		fs.Usage()
		return 2
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	sources, err := readProject(dir)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro check:", err)
		return 1
	}
	p, diags := project.Load(sources...)
	loader := &blueprint.Loader{Builtin: blueprints.FS, ProjectDir: dir}
	if !diags.HasErrors() {
		_, more := blueprint.Resolve(p, loader)
		diags = append(diags, more...)
		diags.Sort()
	}
	if len(diags) > 0 {
		diag.Render(stderr, diags, func(name string) []byte {
			if src := sourceFunc(sources)(name); src != nil {
				return src
			}
			return loader.Source(name)
		})
	}
	if diags.HasErrors() {
		return 1
	}
	fmt.Fprintf(stdout, "ok: app %s (%s, %s, %s, %s)\n", p.App.Raw,
		count(len(sources), "file", "files"),
		count(len(p.Entities), "entity", "entities"),
		count(len(p.Enums), "enum", "enums"),
		count(len(p.App.Blueprints), "blueprint", "blueprints"))
	return 0
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// projectFiles lists the .esq files of the project in dir, sorted, without
// blueprint manifests.
func projectFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.esq"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		if filepath.Base(m) != manifestName {
			files = append(files, m)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .esq files in %s", dir)
	}
	sort.Strings(files)
	return files, nil
}

func readProject(dir string) ([]project.Source, error) {
	files, err := projectFiles(dir)
	if err != nil {
		return nil, err
	}
	sources := make([]project.Source, 0, len(files))
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sources = append(sources, project.Source{Name: f, Src: src})
	}
	return sources, nil
}

func sourceFunc(sources []project.Source) diag.SourceFunc {
	return func(name string) []byte {
		for _, s := range sources {
			if s.Name == name {
				return s.Src
			}
		}
		return nil
	}
}
