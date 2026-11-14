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
)

func runFmt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "", "the .esq file to format (default: the only .esq file in the current directory)")
	write := fs.Bool("w", false, "write the result back to the file instead of printing it")
	check := fs.Bool("check", false, "exit with status 1 if the file is not formatted, without changing it")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro fmt [-w | -check] [-f file.esq]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Formats a .esq file: aligned columns, two-space indentation, comments kept.")
		fmt.Fprintln(stderr, "Prints the result unless -w or -check is given.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "esquadro fmt: unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return 2
	}
	if *write && *check {
		fmt.Fprintln(stderr, "esquadro fmt: -w and -check cannot be used together")
		return 2
	}

	path, err := findProjectFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro fmt:", err)
		return 1
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro fmt:", err)
		return 1
	}
	f, diags := dsl.Parse(path, src)
	if diags.HasErrors() {
		diag.Render(stderr, diags, func(string) []byte { return src })
		return 1
	}
	out := dsl.Format(f)

	switch {
	case *check:
		if !bytes.Equal(out, src) {
			fmt.Fprintf(stderr, "%s is not formatted; run: esquadro fmt -w\n", path)
			return 1
		}
	case *write:
		if bytes.Equal(out, src) {
			return 0
		}
		if err := writeFileAtomic(path, out); err != nil {
			fmt.Fprintln(stderr, "esquadro fmt:", err)
			return 1
		}
	default:
		stdout.Write(out)
	}
	return 0
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
