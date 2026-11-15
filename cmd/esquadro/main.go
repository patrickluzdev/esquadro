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
	// hidden keeps a command out of the help: it is for esquadro's own
	// development.
	hidden bool
}

func commands() []command {
	return []command{
		{"add", "add blueprints to the project", runAdd, false},
		{"blueprint", "list, inspect, update, write and test blueprints", runBlueprint, false},
		{"check", "check the project for errors", runCheck, false},
		{"doctor", "check that the tools the project needs are installed", runDoctor, false},
		{"explain", "tell where a generated file came from", runExplain, false},
		{"fmt", "format .esq files", runFmt, false},
		{"gen", "generate the project", runGen, false},
		{"lsp", "run the language server for editors", runLSP, false},
		{"new", "create a project", runNew, false},
		{"remove", "remove blueprints from the project", runRemove, false},
		{"version", "print the esquadro version", runVersion, false},
		{"docs", "write the reference pages of the documentation site", runDocs, true},
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
		if !c.hidden {
			fmt.Fprintf(w, "  %-10s %s\n", c.name, c.short)
		}
	}
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintln(stdout, "esquadro", version.String())
	return 0
}
