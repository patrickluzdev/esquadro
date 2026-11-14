package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/model"
)

// manifestName is the blueprint manifest file, which is not a project file.
const manifestName = "blueprint.esq"

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "", "the .esq file to check (default: the only .esq file in the current directory)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro check [-f file.esq]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Checks a .esq file for syntax and semantic errors without generating anything.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "esquadro check: unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return 2
	}

	path, err := findProjectFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro check:", err)
		return 1
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro check:", err)
		return 1
	}

	p, diags := model.Load(path, src)
	if len(diags) > 0 {
		diag.Render(stderr, diags, func(string) []byte { return src })
	}
	if diags.HasErrors() {
		return 1
	}
	fmt.Fprintf(stdout, "%s: ok (app %s, %s, %s, %s)\n", path, p.App.Raw,
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

// findProjectFile returns the explicit file, or the only .esq file in the
// current directory (ignoring blueprint manifests).
func findProjectFile(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	matches, err := filepath.Glob("*.esq")
	if err != nil {
		return "", err
	}
	var files []string
	for _, m := range matches {
		if m != manifestName {
			files = append(files, m)
		}
	}
	sort.Strings(files)
	switch len(files) {
	case 0:
		return "", errors.New("no .esq file in the current directory; pass one with -f")
	case 1:
		return files[0], nil
	default:
		return "", fmt.Errorf("found several .esq files (%s); pick one with -f", strings.Join(files, ", "))
	}
}
