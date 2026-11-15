package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/compose"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// A finding of esquadro doctor.
type finding struct {
	level string // ok, info, warn or fail
	what  string
	fix   string
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro doctor [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Checks that the tools esquadro and the project in dir (default: the current")
		fmt.Fprintln(stderr, "directory) need are installed (Go, git, Task, Docker, sqlc), that the")
		fmt.Fprintln(stderr, "project checks, and that its generated code is what the model generates;")
		fmt.Fprintln(stderr, "says how to fix what is not. Outside a project, it checks the tools only.")
	}
	dir, ok := oneDir("doctor", fs, args, stderr)
	if !ok {
		return 2
	}

	// What the project uses decides what it needs.
	var l *loadedProject
	inProject := isProject(dir)
	var findings []finding
	if inProject {
		if l = loadProject("doctor", dir, io.Discard, false); l == nil {
			findings = append(findings, finding{"fail", "the project has errors", "see them with: esquadro check"})
		}
	}
	uses := func(names ...string) bool {
		if l == nil {
			return false
		}
		for _, n := range names {
			if l.set.Get(n) != nil {
				return true
			}
		}
		return false
	}
	fromGit := false
	if l != nil {
		for _, b := range l.set.Blueprints {
			fromGit = fromGit || b.Ref.Source == project.SourceGit
		}
	}
	need := func(needed bool) string {
		if needed {
			return "fail"
		}
		return "warn"
	}

	findings = append(findings, checkGo(dir))
	findings = append(findings, checkTool("git", []string{"--version"}, need(uses("git") || fromGit),
		"blueprints from git and the git blueprint need it", "https://git-scm.com/downloads"))
	findings = append(findings, checkTool("task", []string{"--version"}, "warn",
		"the project's tasks (task run, task test...) need it", "https://taskfile.dev/installation/"))
	dockerNeeded := uses("compose", "testcontainers", "docker")
	docker := checkDocker(need(dockerNeeded))
	findings = append(findings, docker)
	if uses("store") {
		if _, err := exec.LookPath("sqlc"); err == nil {
			findings = append(findings, checkTool("sqlc", []string{"version"}, "fail", "", ""))
		} else if docker.level == "ok" {
			findings = append(findings, finding{"info", "sqlc is not installed: the store blueprint runs it in Docker", ""})
		} else {
			findings = append(findings, finding{"fail", "neither sqlc nor Docker: the store blueprint needs one to generate the queries",
				"install sqlc: https://docs.sqlc.dev/en/latest/overview/install.html"})
		}
	}
	if l != nil {
		findings = append(findings, finding{"ok", "the project checks", ""})
		var out bytes.Buffer
		switch runGen([]string{"-check", dir}, &out, &out) {
		case 0:
			findings = append(findings, finding{"ok", "the generated code is what the model generates", ""})
		default:
			findings = append(findings, finding{"fail", "the generated code is not what the model generates", "see what would change: esquadro gen -dry-run; then: esquadro gen"})
		}
	}

	if inProject {
		abs, _ := filepath.Abs(dir)
		fmt.Fprintf(stdout, "esquadro doctor: the project in %s\n\n", abs)
	} else {
		fmt.Fprintln(stdout, "esquadro doctor: no project here (no esquadro/ folder); checking the tools only")
		fmt.Fprintln(stdout)
	}
	fails, warns := 0, 0
	for _, f := range findings {
		fmt.Fprintf(stdout, "  %-5s %s\n", f.level, f.what)
		if f.fix != "" {
			fmt.Fprintf(stdout, "        %s\n", f.fix)
		}
		switch f.level {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	fmt.Fprintln(stdout)
	switch {
	case fails > 0:
		fmt.Fprintf(stdout, "%s to fix.\n", count(fails, "problem", "problems"))
		return 1
	case warns > 0:
		fmt.Fprintf(stdout, "Ready, with %s.\n", count(warns, "warning", "warnings"))
	default:
		fmt.Fprintln(stdout, "Ready.")
	}
	return 0
}

// checkTool runs a tool to find its version.
func checkTool(name string, args []string, missing, why, install string) finding {
	path, err := exec.LookPath(name)
	if err != nil {
		f := finding{missing, name + " is not installed", "install it: " + install}
		if why != "" {
			f.what += ": " + why
		}
		return f
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		return finding{missing, fmt.Sprintf("%s does not run: %v", name, err), "reinstall it: " + install}
	}
	return finding{"ok", name + " " + versionIn(string(out)), ""}
}

var versionNumber = regexp.MustCompile(`v?(\d+\.\d+(\.\d+)?)`)

// versionIn is the first version number in a tool's output.
func versionIn(out string) string {
	if m := versionNumber.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return strings.TrimSpace(out)
}

var goDirective = regexp.MustCompile(`(?m)^go (\d+\.\d+)`)

// checkGo finds Go, at least the release the project's go.mod, or
// esquadro's generated projects, ask for.
func checkGo(dir string) finding {
	const install = "install it: https://go.dev/dl/"
	path, err := exec.LookPath("go")
	if err != nil {
		return finding{"fail", "go is not installed: the generated projects are Go", install}
	}
	out, err := exec.Command(path, "env", "GOVERSION").Output()
	if err != nil {
		return finding{"fail", fmt.Sprintf("go does not run: %v", err), install}
	}
	have := versionIn(strings.TrimPrefix(strings.TrimSpace(string(out)), "go"))
	want, from := compose.GoVersion(), "the projects esquadro generates need"
	if mod, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		if m := goDirective.FindSubmatch(mod); m != nil {
			want, from = string(m[1]), "go.mod asks for"
		}
	}
	if olderGo(have, want) {
		return finding{"fail", fmt.Sprintf("go %s is older than %s, which %s", have, want, from),
			fmt.Sprintf("install Go %s or later: https://go.dev/dl/", want)}
	}
	return finding{"ok", "go " + have, ""}
}

// olderGo reports whether version a (1.26.2) is older than release b
// (1.27).
func olderGo(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(pa), len(pb), 2) {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return false
}

// checkDocker finds Docker, and whether it runs.
func checkDocker(missing string) finding {
	const why = "compose, testcontainers and the docker blueprint need it"
	path, err := exec.LookPath("docker")
	if err != nil {
		return finding{missing, "docker is not installed: " + why, "install it: https://docs.docker.com/get-docker/"}
	}
	out, err := exec.Command(path, "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return finding{missing, "docker is installed but not running: " + why, "start Docker (Docker Desktop, OrbStack, Colima...)"}
	}
	return finding{"ok", "docker " + strings.TrimSpace(string(out)) + ", running", ""}
}
