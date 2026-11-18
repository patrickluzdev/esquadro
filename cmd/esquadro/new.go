package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// blueprintGroups name what the built-in blueprints are for, as comments
// over them in an app.esq written from a list of blueprints.
var blueprintGroups = map[string]string{
	"app": "base", "env": "base", "domain": "base", "http": "base", "worker": "base", "jobs": "base",
	"postgres": "data", "migrations": "data", "store": "data", "seed": "data", "audit": "data", "testcontainers": "data", "compose": "data",
	"rest": "api", "api_docs": "api", "client_ts": "api", "spa": "api", "security": "api", "mail": "api", "storage": "api", "rate_limit": "api",
	"compression": "api", "i18n": "api",
	"docker": "ship", "github_actions": "ship", "deploy": "ship", "observability": "ship",
	"workspace": "tools", "git": "tools", "branching": "tools",
}

// platformGroups name what the blueprints of a platform are for.
var platformGroups = map[string]string{"server": "servers", "mail": "servers", "caddy": "servers", "postgres": "servers", "apps": "servers", "observability": "servers", "backup": "servers", "cloudflare": "servers", "github_actions": "tools", "git": "tools"}

// groupTitles are the comments over each group.
var groupTitles = map[string]string{
	"base":  "the program, its modules and how it runs",
	"data":  "PostgreSQL: connection, migrations, use cases, tests and local services",
	"api":   "the JSON API and what every request goes through",
	"ship":  "running it in production: telemetry, the container image and the CI",
	"tools": "the repository",
	// A platform's.
	"servers": "the servers, kept by Ansible, and the services they share",
}

var projectName = regexp.MustCompile(`^[a-z][a-z0-9]*([_-][a-z0-9]+)*$`)

func runNew(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(stderr)
	module := flags.String("module", "", "Go module path (default: github.com/<git config github.user>/<name>, or example.com/<name>)")
	starter := flags.String("starter", "", "start from a starter: "+strings.Join(starterNames(), ", "))
	bps := flags.String("blueprints", "", "comma-separated blueprints of the app, instead of a starter (default: app,env)")
	noGit := flags.Bool("no-git", false, "leave out the git blueprint: no repository, no first commit")
	noGen := flags.Bool("no-gen", false, "write the .esq files only, without generating")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro new [-starter name | -blueprints list] [-module path] [-no-git] [-no-gen] <name>")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Creates the folder <name> with esquadro/app.esq and generates the project.")
		fmt.Fprintln(stderr, "Without -starter, the project is the smallest one: the app blueprint, and")
		fmt.Fprintln(stderr, "git, which makes it a repository with a first commit; add blueprints with")
		fmt.Fprintln(stderr, "esquadro add. A starter is a project to begin from, with its blueprints")
		fmt.Fprintln(stderr, "listed in app.esq and an example model:")
		fmt.Fprintln(stderr)
		for _, st := range starterNames() {
			fmt.Fprintf(stderr, "  %-8s %s\n", st, starterAbout(st))
		}
		fmt.Fprintln(stderr)
		flags.PrintDefaults()
	}
	// Flags may come after the name too: esquadro new shop -module x.
	var positional []string
	for rest := args; ; {
		if err := flags.Parse(rest); err != nil {
			return 2
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		rest = flags.Args()[1:]
	}
	if len(positional) != 1 {
		fmt.Fprintf(stderr, "esquadro new: expected the project's name, got %d arguments\n", len(positional))
		flags.Usage()
		return 2
	}
	dir := positional[0]
	name := filepath.Base(dir)
	if !projectName.MatchString(name) {
		fmt.Fprintf(stderr, "esquadro new: %q is not a project name: use lower-case letters and digits, words joined by - or _ (e.g. shop, my-shop)\n", name)
		return 2
	}
	if *starter != "" && *bps != "" {
		fmt.Fprintln(stderr, "esquadro new: give -starter or -blueprints, not both")
		return 2
	}
	if *starter != "" && !slices.Contains(starterNames(), *starter) {
		fmt.Fprintf(stderr, "esquadro new: there is no starter %q; the starters are %s\n", *starter, strings.Join(starterNames(), ", "))
		return 2
	}
	chosen := []string{"app", "env"}
	if *bps != "" {
		chosen = nil
		for b := range strings.SplitSeq(*bps, ",") {
			if b = strings.TrimSpace(b); b != "" {
				chosen = append(chosen, b)
			}
		}
		if len(chosen) == 0 {
			fmt.Fprintln(stderr, "esquadro new: -blueprints is empty")
			return 2
		}
	}
	if *module == "" {
		*module = defaultModule(name)
	}
	if err := emptyDir(dir); err != nil {
		fmt.Fprintf(stderr, "esquadro new: %v\n", err)
		return 1
	}

	var files map[string]string
	if *starter != "" {
		var err error
		if files, err = starterFiles(*starter, name, *module, !*noGit); err != nil {
			fmt.Fprintf(stderr, "esquadro new: %v\n", err)
			return 1
		}
	} else {
		if !*noGit {
			chosen = append(chosen, "git")
		}
		files = map[string]string{"app.esq": appFile(name, *module, chosen, *bps == "")}
	}
	model := filepath.Join(dir, workspace.Dir)
	if err := os.MkdirAll(model, 0o755); err != nil {
		fmt.Fprintf(stderr, "esquadro new: %v\n", err)
		return 1
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(model, file), []byte(content), 0o644); err != nil {
			fmt.Fprintf(stderr, "esquadro new: %v\n", err)
			return 1
		}
	}
	names := slices.Sorted(maps.Keys(files))
	fmt.Fprintf(stdout, "created %s/ with %s (module %s)\n", model, strings.Join(names, ", "), *module)
	if !*noGen {
		if code := runGen([]string{dir}, stdout, stderr); code != 0 {
			fmt.Fprintf(stderr, "esquadro new: the .esq files are in %s/; fix the problem above and run esquadro gen there\n", dir)
			return code
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, nextSteps(dir, files))
	return 0
}

// nextSteps tells what to do with the new project.
func nextSteps(dir string, files map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Next: cd %s\n", dir)
	if _, ok := files["platform.esq"]; ok {
		fmt.Fprintf(&b, "  write your server's address in %s/platform.esq and run esquadro gen\n", workspace.Dir)
		b.WriteString("  task deps, task secrets:init, then task bootstrap; README.md has the rest\n")
		return b.String()
	}
	if len(files) == 1 {
		b.WriteString("  esquadro blueprint list    the blueprints you can add\n")
		b.WriteString("  esquadro add <blueprint>   add one to esquadro/app.esq and generate\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  change the model in %s/ (one .esq file per module) and run esquadro gen\n", workspace.Dir)
	if strings.Contains(files["app.esq"], "blueprint compose") {
		b.WriteString("  task up, task migrate and task run start it locally; README.md has the rest\n")
	}
	if strings.Contains(files["app.esq"], "blueprint git {") {
		b.WriteString("  esquadro add branching    gitflow, or another branching model, for the repository\n")
	}
	return b.String()
}

// defaultModule is github.com/<user>/<name> when git knows the GitHub user.
func defaultModule(name string) string {
	out, err := exec.Command("git", "config", "--get", "github.user").Output()
	if user := string(bytes.TrimSpace(out)); err == nil && user != "" {
		return "github.com/" + user + "/" + name
	}
	return "example.com/" + name
}

// emptyDir creates dir, or accepts it when it exists and is empty.
func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(dir, 0o755)
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("%s already exists and is not empty", dir)
	}
	return nil
}

// appFile is the app.esq of a project made of a list of blueprints; hint
// adds how to add more, for the smallest project.
func appFile(name, module string, chosen []string, hint bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "/// %s, generated by esquadro.\n", label(name))
	fmt.Fprintf(&b, "app %s {\n", strings.ReplaceAll(name, "-", "_"))
	fmt.Fprintf(&b, "  module %q\n", module)
	group := "-"
	if hint {
		b.WriteString("\n")
	}
	for _, bp := range chosen {
		if g := blueprintGroups[bp]; !hint && g != group {
			group = g
			b.WriteString("\n")
			if title := groupTitles[g]; title != "" {
				fmt.Fprintf(&b, "  // %s\n", title)
			}
		}
		if bp == "git" {
			// Written out so the option shows: false keeps the repository
			// without the first commit.
			b.WriteString("  blueprint git {\n    initial_commit true\n  }\n")
			continue
		}
		fmt.Fprintf(&b, "  blueprint %s\n", bp)
	}
	if hint {
		b.WriteString("\n  // Add more with esquadro add <blueprint>; see them with esquadro blueprint list.\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// label is the project's name as words: my-shop is My shop.
func label(name string) string {
	l := strings.NewReplacer("-", " ", "_", " ").Replace(name)
	return strings.ToUpper(l[:1]) + l[1:]
}
