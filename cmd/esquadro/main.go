// Command esquadro generates Go projects from a .esq model and a set of
// blueprints (template plugins).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/patrickluzdev/esquadro/internal/version"
)

type command struct {
	name  string
	short string
	run   func(args []string, stdout, stderr io.Writer) int
}

func commands() []command {
	return []command{
		{"check", "check a .esq file for errors", runCheck},
		{"version", "print the esquadro version", runVersion},
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(stdout)
		return 0
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "esquadro: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: esquadro <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.short)
	}
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintln(stdout, "esquadro", version.String())
	return 0
}
