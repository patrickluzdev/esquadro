package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

func runRemove(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	noGen := fs.Bool("no-gen", false, "change app.esq only, without generating")
	force := fs.Bool("force", false, "delete the blueprint's generated files even if you edited them")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: esquadro remove [-no-gen] [-force] <blueprint>[,<blueprint>...] [dir]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Removes blueprints from the app block of the project in dir (default: the")
		fmt.Fprintln(stderr, "current directory) and generates: the files they generated go, unless you")
		fmt.Fprintln(stderr, "edited them (then nothing is written, as with gen); the files esquadro wrote")
		fmt.Fprintln(stderr, "once for you stay. A blueprint another one requires is not removed. Give")
		fmt.Fprintln(stderr, "a blueprint by its name, as esquadro blueprint list shows it.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	names, dir, ok := parseManageArgs("remove", fs, args, stderr)
	if !ok {
		return 2
	}
	l := loadProject("remove", dir, stderr, false)
	if l == nil {
		return 1
	}
	app, err := findApp(l.sources)
	if err != nil {
		fmt.Fprintf(stderr, "esquadro remove: %v\n", err)
		return 1
	}

	var removed []*blueprint.Blueprint
	for _, name := range names {
		b := l.set.Get(name)
		if b == nil {
			fmt.Fprintf(stderr, "esquadro remove: the app has no blueprint %s\n", name)
			return 1
		}
		removed = append(removed, b)
	}
	gone := func(name string) bool { return slices.Contains(names, name) }
	for _, b := range l.set.Blueprints {
		if gone(b.Manifest.Name) {
			continue
		}
		for _, dep := range b.Manifest.Requires {
			if gone(dep.Name) {
				fmt.Fprintf(stderr, "esquadro remove: %s requires %s: remove %s too, or keep %s\n",
					b.Manifest.Name, dep.Name, b.Manifest.Name, dep.Name)
				return 1
			}
		}
	}

	src := app.src
	var cuts [][2]int
	for _, b := range removed {
		for _, item := range app.decl.Items {
			if r, ok := item.(*dsl.BlueprintRef); ok && r.Ref.Span == b.Ref.RefSpan {
				cuts = append(cuts, lineRange(src, r.Span))
			}
		}
	}
	slices.SortFunc(cuts, func(a, b [2]int) int { return b[0] - a[0] })
	changed := slices.Clone(src)
	for _, c := range cuts {
		changed = cutLines(changed, c[0], c[1])
	}
	if loadSources("remove", dir, withSource(l.sources, app.name, changed), stderr, false) == nil {
		fmt.Fprintln(stderr, "esquadro remove: app.esq was not changed")
		return 1
	}
	if err := os.WriteFile(app.name, changed, 0o644); err != nil {
		fmt.Fprintf(stderr, "esquadro remove: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "removed %s from app.esq\n", strings.Join(names, ", "))
	if unused := unusedDependencies(l.set, names); len(unused) > 0 {
		fmt.Fprintf(stdout, "no blueprint requires %s any more: esquadro remove %s, if you do not use it\n",
			strings.Join(unused, ", "), strings.Join(unused, ","))
	}
	if *noGen {
		return 0
	}
	genArgs := []string{dir}
	if *force {
		genArgs = []string{"-force", dir}
	}
	return runGen(genArgs, stdout, stderr)
}

// lineRange is the span's whole lines, with their line break.
func lineRange(src []byte, s dsl.Span) [2]int {
	start := bytes.LastIndexByte(src[:s.Pos.Offset], '\n') + 1
	end := len(src)
	if i := bytes.IndexByte(src[s.End.Offset:], '\n'); i >= 0 {
		end = s.End.Offset + i + 1
	}
	return [2]int{start, end}
}

// cutLines removes src[start:end], whole lines, and the comment over a
// group of blueprints that the cut leaves empty: a // line between a blank
// line and a blank line or the end of the block, with the blank line before
// it.
func cutLines(src []byte, start, end int) []byte {
	out := slices.Concat(src[:start], src[end:])
	lineBefore := func(at int) (int, string) {
		if at == 0 {
			return -1, ""
		}
		from := bytes.LastIndexByte(out[:at-1], '\n') + 1
		return from, strings.TrimSpace(string(out[from:at]))
	}
	next := ""
	if i := bytes.IndexByte(out[start:], '\n'); i >= 0 {
		next = strings.TrimSpace(string(out[start : start+i]))
	}
	from, comment := lineBefore(start)
	if from < 0 || !strings.HasPrefix(comment, "//") || strings.HasPrefix(comment, "///") || (next != "" && next != "}") {
		return out
	}
	blank, text := lineBefore(from)
	if blank < 0 || text != "" {
		return out
	}
	if next == "}" {
		// No blank line before the closing brace.
		return slices.Concat(out[:blank], out[start:])
	}
	return slices.Concat(out[:from], out[start:])
}

// unusedDependencies are the blueprints the removed ones required that no
// remaining blueprint requires.
func unusedDependencies(set *blueprint.Set, removed []string) []string {
	required := map[string]bool{}
	for _, b := range set.Blueprints {
		if slices.Contains(removed, b.Manifest.Name) {
			continue
		}
		for _, d := range b.Manifest.Requires {
			required[d.Name] = true
		}
	}
	var unused []string
	for _, b := range set.Blueprints {
		if !slices.Contains(removed, b.Manifest.Name) {
			continue
		}
		for _, d := range b.Manifest.Requires {
			if !required[d.Name] && !slices.Contains(removed, d.Name) && d.Name != "app" && !slices.Contains(unused, d.Name) {
				unused = append(unused, d.Name)
			}
		}
	}
	return unused
}
