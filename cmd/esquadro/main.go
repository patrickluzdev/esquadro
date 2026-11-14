// Command esquadro generates Go projects from a .esq model and a set of
// plantas (template plugins).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/patrickluz/esquadro/internal/version"
)

type command struct {
	name  string
	short string
	run   func(args []string, stdout, stderr io.Writer) int
}

func commands() []command {
	return []command{
		{"version", "mostra a versão do esquadro", runVersion},
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
	fmt.Fprintf(stderr, "esquadro: comando desconhecido %q\n\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Uso: esquadro <comando> [argumentos]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Comandos:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.short)
	}
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintln(stdout, "esquadro", version.String())
	return 0
}
