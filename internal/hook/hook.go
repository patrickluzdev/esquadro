// Package hook runs the programs blueprints declare to run before (pre-gen)
// or after (post-gen) generation.
//
// A hook reads one JSON Input on stdin and writes one JSON Output on stdout;
// its stderr is shown to the user. It runs in the project folder. A pre-gen
// hook may return files, which join the generated ones; it must not write to
// disk. A post-gen hook may write files, but only those matching the globs
// its manifest declares with writes. Hooks of built-in blueprints are Go
// functions that speak the same protocol.
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/compose"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

// Input is what a hook receives on stdin.
type Input struct {
	Event     string              `json:"event"`
	Blueprint string              `json:"blueprint"`
	Project   project.Description `json:"project"`
	// Options are the blueprint's options, defaults included.
	Options map[string]any `json:"options"`
	// Blueprints are the options of every blueprint of the app, by name:
	// a blueprint that works on another one's data reads its settings here
	// (e.g. the schemas of postgres).
	Blueprints map[string]map[string]any `json:"blueprints"`
	// Files are the paths generated from templates in this run.
	Files []string `json:"files"`
	// GoRequires are the Go modules every blueprint of the project needs.
	GoRequires []GoRequire `json:"go_requires"`
	// ProjectDir is the project folder, absolute; external hooks also run
	// in it.
	ProjectDir string `json:"project_dir"`
	// Previous is the project as the last successful generation saw it, or
	// null for the first one: what a hook compares the project with to
	// write, for example, a migration.
	Previous *project.Description `json:"previous"`
	DryRun   bool                 `json:"dry_run"`
}

// GoRequire is a Go module a blueprint needs, with its minimum version.
type GoRequire struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}

// Output is what a hook writes on stdout. An empty stdout is an empty
// Output.
type Output struct {
	Messages []Message `json:"messages,omitempty"`
	// Files are returned by pre-gen hooks; post-gen hooks write to disk.
	Files []File `json:"files,omitempty"`
}

// Message is shown to the user.
type Message struct {
	Level string `json:"level"` // info, warning or error
	Text  string `json:"text"`
}

// File is a file a pre-gen hook adds to the generation.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	// Once makes the file the user's, like a .once template: created when
	// missing, never updated or deleted.
	Once bool `json:"once,omitempty"`
}

// Func is a hook built into esquadro.
type Func func(ctx context.Context, in Input) (Output, error)

// Runner runs the hooks of a project's blueprints.
type Runner struct {
	ProjectDir string
	// Builtin holds the hooks of built-in blueprints, by "blueprint/path".
	Builtin map[string]Func
	// Out receives the hooks' messages and stderr.
	Out io.Writer
	// Previous is the project of the last successful generation, if any.
	Previous *project.Description
}

// Key names a built-in hook in Runner.Builtin.
func Key(bp, path string) string { return bp + "/" + path }

// ID names a hook in messages and in the lockfile.
func ID(bp *blueprint.Blueprint, h *blueprint.Hook) string {
	return bp.Manifest.Name + " " + h.Event + " " + h.Path
}

// Hooks returns the hooks of an event in generation order, except that
// post-gen hooks that write files (generators, such as sqlc) run first, then
// those that only edit files (tidy-ups, such as go mod tidy), which need the
// generated code to be there, and last those that change no file of the
// project (such as a commit), which need it finished.
func Hooks(set *blueprint.Set, event string) []Hooked {
	var writers, editors, others []Hooked
	for _, bp := range set.Blueprints {
		for _, h := range bp.Manifest.Hooks {
			switch {
			case h.Event != event:
			case event == "post-gen" && len(h.Writes) == 0 && len(h.Edits) > 0:
				editors = append(editors, Hooked{bp, h})
			case event == "post-gen" && len(h.Writes) == 0:
				others = append(others, Hooked{bp, h})
			default:
				writers = append(writers, Hooked{bp, h})
			}
		}
	}
	return slices.Concat(writers, editors, others)
}

// Hooked is a hook with its blueprint.
type Hooked struct {
	Blueprint *blueprint.Blueprint
	Hook      *blueprint.Hook
}

// CheckOverlaps reports generated files inside a post-gen hook's writes:
// a template and a hook must never fight over a file.
func CheckOverlaps(set *blueprint.Set, res *compose.Result) diag.List {
	var diags diag.List
	for _, hk := range Hooks(set, "post-gen") {
		for _, f := range res.Files {
			if MatchAny(hk.Hook.Writes, f.Path) || MatchAny(hk.Hook.Edits, f.Path) {
				diags.Errorf(hk.Hook.Span.Pos, hk.Hook.Span.End,
					"hook %s may change %s, which blueprint %s also generates from %s", ID(hk.Blueprint, hk.Hook), f.Path, f.Blueprint, f.Template)
			}
		}
	}
	return diags
}

// PreGen runs the pre-gen hooks and adds the files they return to res.
func (r *Runner) PreGen(ctx context.Context, p *project.Project, set *blueprint.Set, res *compose.Result, dryRun bool) error {
	for _, hk := range Hooks(set, "pre-gen") {
		out, err := r.run(ctx, hk, r.input(p, set, res, hk, dryRun))
		if err != nil {
			return err
		}
		id := ID(hk.Blueprint, hk.Hook)
		for _, f := range out.Files {
			target, ok := compose.CleanTarget(f.Path)
			if !ok {
				return fmt.Errorf("hook %s returned %q, which is not a path inside the project", id, f.Path)
			}
			if prev := res.File(target); prev != nil {
				return fmt.Errorf("hook %s returned %s, which blueprint %s already generates from %s", id, target, prev.Blueprint, prev.Template)
			}
			res.Files = append(res.Files, &compose.File{
				Path: target, Content: []byte(f.Content), Blueprint: hk.Blueprint.Manifest.Name,
				Template: "hook " + hk.Hook.Path, Once: f.Once,
			})
		}
	}
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return nil
}

// PostGen runs the post-gen hooks after the generated files are on disk. It
// checks that each hook only touched files matching its writes and records
// those files in the lockfile, so later generations can tell if the user
// edited them.
func (r *Runner) PostGen(ctx context.Context, p *project.Project, set *blueprint.Set, res *compose.Result) error {
	for _, hk := range Hooks(set, "post-gen") {
		id := ID(hk.Blueprint, hk.Hook)
		before, err := stamps(r.ProjectDir)
		if err != nil {
			return err
		}
		if _, err := r.run(ctx, hk, r.input(p, set, res, hk, false)); err != nil {
			return err
		}
		after, err := stamps(r.ProjectDir)
		if err != nil {
			return err
		}
		var outside []string
		allowed := append(slices.Clone(hk.Hook.Writes), hk.Hook.Edits...)
		for _, path := range changed(before, after) {
			if !MatchAny(allowed, path) {
				outside = append(outside, path)
			}
		}
		if len(outside) > 0 {
			if len(outside) > 5 {
				outside = append(outside[:5], fmt.Sprintf("and %d more", len(outside)-5))
			}
			return fmt.Errorf("hook %s changed files outside what it declares in writes and edits (%s): %s",
				id, strings.Join(allowed, ", "), strings.Join(outside, ", "))
		}
		if err := workspace.TrackHookFiles(r.ProjectDir, id, hk.Blueprint.Manifest.Name, hk.Hook.Writes, MatchAny); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) input(p *project.Project, set *blueprint.Set, res *compose.Result, hk Hooked, dryRun bool) Input {
	files := make([]string, len(res.Files))
	for i, f := range res.Files {
		files[i] = f.Path
	}
	reqs := []GoRequire{}
	for _, bp := range set.Blueprints {
		for _, r := range bp.Manifest.GoRequires {
			reqs = append(reqs, GoRequire{Module: r.Module, Version: r.Version})
		}
	}
	dir, err := filepath.Abs(r.ProjectDir)
	if err != nil {
		dir = r.ProjectDir
	}
	return Input{
		Event: hk.Hook.Event, Blueprint: hk.Blueprint.Manifest.Name, Project: p.Describe(),
		Options: set.Options[hk.Blueprint.Manifest.Name], Blueprints: set.Options, Files: files, GoRequires: reqs,
		ProjectDir: dir, Previous: r.Previous, DryRun: dryRun,
	}
}

// run runs one hook and prints its messages.
func (r *Runner) run(ctx context.Context, hk Hooked, in Input) (Output, error) {
	id := ID(hk.Blueprint, hk.Hook)
	var out Output
	var err error
	if hk.Blueprint.Location == "built-in" {
		fn := r.Builtin[Key(hk.Blueprint.Manifest.Name, hk.Hook.Path)]
		if fn == nil {
			return Output{}, fmt.Errorf("hook %s is not built into this esquadro", id)
		}
		// Round-trip through JSON so built-in hooks see exactly what
		// external ones do.
		data, _ := json.Marshal(in)
		var copied Input
		if err := json.Unmarshal(data, &copied); err != nil {
			return Output{}, err
		}
		out, err = fn(ctx, copied)
	} else {
		out, err = r.exec(ctx, hk, in)
	}
	for _, m := range out.Messages {
		level := m.Level
		if level == "" {
			level = "info"
		}
		fmt.Fprintf(r.Out, "%s: %s: %s\n", id, level, m.Text)
	}
	if err != nil {
		return out, fmt.Errorf("hook %s failed: %w", id, err)
	}
	return out, nil
}

const maxOutput = 64 << 20

func (r *Runner) exec(ctx context.Context, hk Hooked, in Input) (Output, error) {
	path := filepath.Join(hk.Blueprint.Dir, filepath.FromSlash(hk.Hook.Path))
	info, err := os.Stat(path)
	if err != nil {
		return Output{}, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return Output{}, fmt.Errorf("%s is not executable (run: chmod +x %s)", path, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Output{}, err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return Output{}, err
	}
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, abs)
	cmd.Dir = r.ProjectDir
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = &limited{w: &stdout, n: maxOutput}
	cmd.Stderr = &prefixed{w: r.Out, prefix: ID(hk.Blueprint, hk.Hook) + ": "}
	runErr := cmd.Run()

	var out Output
	if text := bytes.TrimSpace(stdout.Bytes()); len(text) > 0 {
		if err := json.Unmarshal(text, &out); err != nil {
			first, _, _ := strings.Cut(string(text), "\n")
			return Output{}, fmt.Errorf("its output is not the expected JSON (%v); it starts with: %.80s", err, first)
		}
	}
	return out, runErr
}

// limited stops writing after n bytes.
type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if len(p) > l.n {
		return 0, fmt.Errorf("output larger than %d bytes", maxOutput)
	}
	l.n -= len(p)
	return l.w.Write(p)
}

// prefixed writes each line with a prefix.
type prefixed struct {
	w      io.Writer
	prefix string
	mid    bool // in the middle of a line
}

func (p *prefixed) Write(b []byte) (int, error) {
	for _, line := range bytes.SplitAfter(b, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if !p.mid {
			io.WriteString(p.w, p.prefix)
		}
		p.w.Write(line)
		p.mid = line[len(line)-1] != '\n'
	}
	return len(b), nil
}

// stamp identifies a file version cheaply.
type stamp struct {
	size int64
	mod  int64
}

// stamps lists the project's files, skipping .git and esquadro.
func stamps(dir string) (map[string]stamp, error) {
	out := map[string]stamp{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == workspace.Dir {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[rel] = stamp{info.Size(), info.ModTime().UnixNano()}
		return nil
	})
	return out, err
}

// changed lists the paths created, modified or removed between two stamps.
func changed(before, after map[string]stamp) []string {
	var out []string
	for p, s := range after {
		if b, ok := before[p]; !ok || b != s {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
