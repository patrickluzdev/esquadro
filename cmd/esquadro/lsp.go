package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/lsp"
	"github.com/patrickluzdev/esquadro/internal/version"
)

func runLSP(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro lsp")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Runs the language server an editor starts for .esq files: errors while")
		fmt.Fprintln(stderr, "you type, the same esquadro check reports, and formatting as esquadro fmt")
		fmt.Fprintln(stderr, "does. It speaks the Language Server Protocol on stdin and stdout.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cache, err := blueprint.DefaultCacheDir()
	if err != nil {
		fmt.Fprintln(stderr, "esquadro lsp:", err)
		return 1
	}
	s := &lsp.Server{Version: version.String(), CacheDir: cache, Delay: 200 * time.Millisecond, Log: stderr}
	if err := s.Serve(os.Stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "esquadro lsp:", err)
		return 1
	}
	return 0
}
