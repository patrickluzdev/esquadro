package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/patrickluzdev/esquadro/internal/compose"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// envCommands are the subcommands of esquadro env.
func envCommands() []command {
	return []command{
		{"list", "list the project's environment variables, by environment", runEnvList, false},
		{"check", "check an environment's .env file against the model", runEnvCheck, false},
	}
}

func runEnv(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stdout, "Usage: esquadro env <command> [arguments]")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Commands:")
		for _, c := range envCommands() {
			fmt.Fprintf(stdout, "  %-6s %s\n", c.name, c.short)
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Variables are declared in the model: by blueprints, and in app.esq's env blocks.")
		return 0
	}
	for _, c := range envCommands() {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "esquadro env: unknown command %q; the commands are list and check\n", args[0])
	return 2
}

// projectEnv loads the project and resolves its variables, their values
// rendered.
func projectEnv(cmd, dir string, stderr io.Writer) (*compose.Result, bool) {
	l := loadProject(cmd, dir, stderr, false)
	if l == nil {
		return nil, false
	}
	res, templates, diags := compose.ComposeWith(l.project, l.set, builtinFuncs)
	if diags.HasErrors() {
		diag.Render(stderr, diags, l.source(templates.Get))
		return nil, false
	}
	return res, true
}

func runEnvList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("env list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro env list [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Lists the environment variables of the project in dir (default: the current")
		fmt.Fprintln(stderr, "directory): their type, the environments that require them, their value in")
		fmt.Fprintln(stderr, "each environment from the model, and the blueprint that declares them.")
	}
	dir, ok := oneDir("env list", fs, args, stderr)
	if !ok {
		return 2
	}
	res, ok := projectEnv("env list", dir, stderr)
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "Environments: %s (APP_ENV picks one; %s is the default)\n\n", strings.Join(res.Environments, ", "), res.Environments[0])
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VARIABLE\tTYPE\tREQUIRED IN\tVALUES\tFROM")
	for _, v := range res.Env {
		typ := string(v.Type)
		if v.Secret {
			typ += ", secret"
		}
		required := strings.Join(v.Required, ", ")
		if required == "" {
			required = "-"
		}
		var values []string
		for _, e := range res.Environments {
			val, ok := v.Values[e]
			switch {
			case v.Generate > 0 && e == res.Environments[0]:
				values = append(values, e+": generated")
			case !ok:
			case v.Secret:
				values = append(values, e+": set")
			default:
				values = append(values, e+": "+val)
			}
		}
		if len(values) == len(res.Environments) && !v.Secret && allSame(v.Values, res.Environments) {
			values = []string{v.Values[res.Environments[0]]}
		}
		shown := strings.Join(values, "; ")
		if shown == "" {
			shown = "-"
		}
		from := v.Blueprint
		if from == "" {
			from = "app.esq"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Name, typ, required, shown, from)
	}
	if err := w.Flush(); err != nil {
		return 1
	}
	return 0
}

func allSame(values map[string]string, envs []string) bool {
	for _, e := range envs[1:] {
		if values[e] != values[envs[0]] {
			return false
		}
	}
	return true
}

var envLine = regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

func runEnvCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("env check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro env check [environment] [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Checks the .env.<environment> file of the project in dir (default: the")
		fmt.Fprintln(stderr, "current directory), or of every environment, against the model: the")
		fmt.Fprintln(stderr, "variables the environment requires that have no value, values of the wrong")
		fmt.Fprintln(stderr, "type, and variables the model does not declare. The process's environment")
		fmt.Fprintln(stderr, "is not read: it checks what the file and the model give.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var only, dir = "", "."
	switch fs.NArg() {
	case 0:
	case 1:
		if isProject(fs.Arg(0)) {
			dir = fs.Arg(0)
		} else {
			only = fs.Arg(0)
		}
	case 2:
		only, dir = fs.Arg(0), fs.Arg(1)
	default:
		fs.Usage()
		return 2
	}
	res, ok := projectEnv("env check", dir, stderr)
	if !ok {
		return 1
	}
	envs := res.Environments
	if only != "" {
		if !slices.Contains(envs, only) {
			fmt.Fprintf(stderr, "esquadro env check: %s is not an environment: %s\n", only, strings.Join(envs, ", "))
			return 2
		}
		envs = []string{only}
	}
	problems := 0
	for _, env := range envs {
		name := ".env." + env
		file, lines, err := readEnvFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "esquadro env check: %v\n", err)
			return 1
		}
		var found []string
		for _, v := range res.Env {
			value := file[v.Name]
			if value == "" {
				value = v.Values[env]
			}
			if value == "" {
				if slices.Contains(v.Required, env) && !(v.Generate > 0 && env == res.Environments[0]) {
					found = append(found, fmt.Sprintf("%s is required in %s and has no value", v.Name, env))
				}
				continue
			}
			if msg := envTypeProblem(value, v.Type); msg != "" {
				shown := fmt.Sprintf(" (%q)", value)
				if v.Secret {
					shown = ""
				}
				found = append(found, fmt.Sprintf("%s:%d: %s must be %s%s", name, lines[v.Name], v.Name, msg, shown))
			}
		}
		declared := map[string]bool{}
		for _, v := range res.Env {
			declared[v.Name] = true
		}
		for _, n := range slices.Sorted(func(yield func(string) bool) {
			for k := range file {
				if !yield(k) {
					return
				}
			}
		}) {
			if !declared[n] && n != "APP_ENV" {
				found = append(found, fmt.Sprintf("%s:%d: %s is not declared: the app does not read it", name, lines[n], n))
			}
		}
		if os.IsNotExist(err) {
			fmt.Fprintf(stdout, "%s: no %s, only the model's values\n", env, name)
		}
		if len(found) == 0 {
			fmt.Fprintf(stdout, "%s: ok\n", env)
			continue
		}
		problems += len(found)
		for _, f := range found {
			fmt.Fprintf(stdout, "%s: %s\n", env, f)
		}
	}
	if problems > 0 {
		fmt.Fprintf(stderr, "\nesquadro env check: %s\n", count(problems, "problem", "problems"))
		return 1
	}
	return 0
}

// readEnvFile reads a .env file: its values, and the line of each.
func readEnvFile(path string) (map[string]string, map[string]int, error) {
	values, lines := map[string]string{}, map[string]int{}
	data, err := os.ReadFile(path)
	if err != nil {
		return values, lines, err
	}
	s := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; s.Scan(); n++ {
		line := strings.TrimSpace(s.Text())
		m := envLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := strings.TrimSpace(m[2])
		if unquoted, err := strconv.Unquote(value); err == nil && strings.HasPrefix(value, `"`) {
			value = unquoted
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		values[m[1]], lines[m[1]] = value, n
	}
	return values, lines, s.Err()
}

// envTypeProblem says what a value should be for its type, or nothing.
func envTypeProblem(value string, t project.EnvType) string {
	var err error
	switch t {
	case project.EnvInt:
		if _, err = strconv.Atoi(value); err != nil {
			return "a whole number"
		}
	case project.EnvNumber:
		if _, err = strconv.ParseFloat(value, 64); err != nil {
			return "a number"
		}
	case project.EnvBool:
		if _, err = strconv.ParseBool(value); err != nil {
			return "true or false"
		}
	case project.EnvDuration:
		if _, err = time.ParseDuration(value); err != nil {
			return "a duration, such as 30s"
		}
	}
	return ""
}
