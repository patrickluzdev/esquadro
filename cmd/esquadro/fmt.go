package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

func runFmt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("w", false, "write the result back to the files instead of printing it")
	check := fs.Bool("check", false, "list files that are not formatted and exit with status 1, without changing them")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro fmt [-w | -check] [path ...]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Formats .esq files: aligned columns, two-space indentation, comments kept.")
		fmt.Fprintln(stderr, "A path can be a file or a directory (its .esq files, not subdirectories).")
		fmt.Fprintln(stderr, "The default path is the project's esquadro/ folder, or the current directory")
		fmt.Fprintln(stderr, "without one. Prints the result unless -w or -check is given.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *write && *check {
		fmt.Fprintln(stderr, "esquadro fmt: -w and -check cannot be used together")
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
		if info, err := os.Stat(workspace.Dir); err == nil && info.IsDir() {
			paths = []string{workspace.Dir}
		}
	}

	files, err := expandPaths(paths)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro fmt:", err)
		return 1
	}
	code := 0
	for _, path := range files {
		if !formatFile(path, *write, *check, stdout, stderr) {
			code = 1
		}
	}
	return code
}

// expandPaths turns directories into their .esq files (blueprint manifests
// included: they use the same syntax) and keeps files as given.
func expandPaths(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		m, err := filepath.Glob(filepath.Join(p, "*.esq"))
		if err != nil {
			return nil, err
		}
		if len(m) == 0 {
			return nil, fmt.Errorf("no .esq files in %s", p)
		}
		files = append(files, m...)
	}
	return files, nil
}

// formatFile formats one file and reports whether it succeeded (and, with
// check, whether it was already formatted).
func formatFile(path string, write, check bool, stdout, stderr io.Writer) bool {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro fmt:", err)
		return false
	}
	f, diags := dsl.Parse(path, src)
	if diags.HasErrors() {
		diag.Render(stderr, diags, func(string) []byte { return src })
		return false
	}
	out := dsl.Format(f)
	switch {
	case check:
		if !bytes.Equal(out, src) {
			fmt.Fprintf(stderr, "%s is not formatted; run: esquadro fmt -w\n", path)
			return false
		}
	case write:
		if !bytes.Equal(out, src) {
			if err := writeFileAtomic(path, out); err != nil {
				fmt.Fprintln(stderr, "esquadro fmt:", err)
				return false
			}
		}
	default:
		stdout.Write(out)
	}
	return true
}

// writeFileAtomic replaces path with data through a temporary file in the
// same directory, so an interrupted write never leaves a truncated file.
func writeFileAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
