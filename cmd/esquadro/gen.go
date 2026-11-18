package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/compose"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/hook"
	"github.com/patrickluzdev/esquadro/internal/version"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

func runGen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "show what would change, with diffs, without writing anything")
	check := fs.Bool("check", false, "write nothing, and exit with status 1 if the files are not what the model generates (for CI)")
	force := fs.Bool("force", false, "overwrite or delete generated files you edited")
	update := fs.Bool("update", false, "resolve the versions of blueprints from git again, instead of the commits the lockfile pins")
	trust := fs.Bool("trust", false, "let the hooks of blueprints from git run without asking")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro gen [-dry-run | -check] [-force] [-update] [-trust] [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Generates the project in dir (default: the current directory) from its .esq")
		fmt.Fprintln(stderr, "files and blueprints. Generated files you edited are never overwritten")
		fmt.Fprintln(stderr, "unless -force is given; if any would be, nothing is written. Blueprints from")
		fmt.Fprintln(stderr, "git stay at the commit the lockfile pins until -update; their hooks run")
		fmt.Fprintln(stderr, "only once you allow them, for each commit.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "esquadro gen: expected one directory, got %d arguments\n", fs.NArg())
		fs.Usage()
		return 2
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	l := loadProject("gen", dir, stderr, *update)
	if l == nil {
		return 1
	}
	trusted, err := trustHooks(l.set, l.lock, *trust, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		return 1
	}
	res, templates, diags := compose.ComposeWith(l.project, l.set, builtinFuncs)
	diags = append(diags, hook.CheckOverlaps(l.set, res)...)
	diags.Sort()
	if len(diags) > 0 {
		diag.Render(stderr, diags, l.source(templates.Get))
	}
	if diags.HasErrors() {
		return 1
	}

	previous, err := workspace.ReadSnapshot(dir)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		return 1
	}
	ctx := context.Background()
	runner := &hook.Runner{ProjectDir: dir, Builtin: builtinHooks, Out: stderr, Previous: previous}
	if err := runner.PreGen(ctx, l.project, l.set, res, *dryRun || *check); err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		return 1
	}

	lock, err := workspace.ReadLock(dir)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		return 1
	}
	plan, err := workspace.MakePlan(dir, res, lock)
	if err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		return 1
	}
	printPlan(stdout, plan, *dryRun)
	if *check {
		n := plan.Count()
		if pending := n[workspace.Create] + n[workspace.Update] + n[workspace.Merge] + n[workspace.Delete] + n[workspace.Conflict]; pending > 0 {
			fmt.Fprintf(stderr, "\nesquadro gen -check: the files are not what the model generates (%s); run esquadro gen\n", count(pending, "change", "changes"))
			return 1
		}
		return 0
	}
	for _, hk := range hook.Hooks(l.set, "post-gen") {
		changes := "changes no file of the project"
		if globs := append(slices.Clone(hk.Hook.Writes), hk.Hook.Edits...); len(globs) > 0 {
			changes = "may change " + strings.Join(globs, ", ")
		}
		fmt.Fprintf(stdout, "%-9s %s  (after writing; %s)\n", "run", hook.ID(hk.Blueprint, hk.Hook), changes)
	}

	conflicts := plan.Conflicts()
	if *dryRun {
		if len(conflicts) > 0 && !*force {
			fmt.Fprintln(stdout, "\nA real run would stop here: see the conflicts above.")
		}
		return 0
	}
	if len(conflicts) > 0 && !*force {
		fmt.Fprintf(stderr, "\nNothing was written: %s would overwrite your work.\n", count(len(conflicts), "change", "changes"))
		fmt.Fprintln(stderr, "Move your changes to a file esquadro does not generate, or run: esquadro gen -force")
		fmt.Fprintln(stderr, "To see the differences: esquadro gen -dry-run")
		return 1
	}

	var bps []workspace.Blueprint
	for _, b := range l.set.Blueprints {
		bps = append(bps, workspace.Blueprint{
			Name: b.Manifest.Name, Version: b.Manifest.Version, Location: b.Location,
			Source: b.Source, Commit: b.Commit, Trusted: trusted[b.Manifest.Name],
		})
	}
	if err := workspace.Write(dir, plan, *force, version.String(), bps); err != nil {
		if errors.Is(err, workspace.ErrConflicts) {
			fmt.Fprintln(stderr, "esquadro gen: nothing was written: there are conflicts")
		} else {
			fmt.Fprintln(stderr, "esquadro gen:", err)
		}
		return 1
	}
	// A merge that left conflicts: the user resolves them, as with git.
	for _, a := range plan.Actions {
		if a.Change == workspace.Merge && a.Conflicts > 0 {
			fmt.Fprintf(stderr, "esquadro gen: %s: %s between <<<<<<< yours and >>>>>>> esquadro; resolve and remove the markers\n", a.Path, count(a.Conflicts, "conflict", "conflicts"))
		}
	}
	// The files, migrations included, now follow this project: the next
	// generation compares with it. Saved before the post-gen hooks, which
	// may commit it; they get the previous project from memory.
	if err := workspace.WriteSnapshot(dir, l.project.Describe()); err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		return 1
	}
	if err := runner.PostGen(ctx, l.project, l.set, res); err != nil {
		fmt.Fprintln(stderr, "esquadro gen:", err)
		fmt.Fprintln(stderr, "The generated files were written; fix the hook and run esquadro gen again.")
		return 1
	}
	return 0
}

// printPlan lists every change except unchanged files, then a summary; with
// diffs, it also shows what changes in updated, deleted and conflicting
// files.
func printPlan(w io.Writer, plan *workspace.Plan, diffs bool) {
	for _, a := range plan.Actions {
		if a.Change == workspace.Unchanged {
			continue
		}
		line := fmt.Sprintf("%-9s %s", a.Change, a.Path)
		if a.Reason != "" {
			line += "  (" + a.Reason + ")"
		}
		fmt.Fprintln(w, line)
	}
	n := plan.Count()
	fmt.Fprintf(w, "%d created, %d updated, %d deleted, %d unchanged",
		n[workspace.Create], n[workspace.Update], n[workspace.Delete], n[workspace.Unchanged])
	if n[workspace.Merge] > 0 {
		fmt.Fprintf(w, ", %d merged", n[workspace.Merge])
	}
	if n[workspace.Keep] > 0 {
		fmt.Fprintf(w, ", %d kept", n[workspace.Keep])
	}
	if n[workspace.Conflict] > 0 {
		fmt.Fprintf(w, ", %s", count(n[workspace.Conflict], "conflict", "conflicts"))
	}
	fmt.Fprintln(w)
	if !diffs {
		return
	}
	for _, a := range plan.Actions {
		switch a.Change {
		case workspace.Update, workspace.Merge, workspace.Delete, workspace.Conflict:
			fmt.Fprintln(w)
			fmt.Fprint(w, workspace.Diff(a.Path, a.Old, a.New))
		}
	}
}
