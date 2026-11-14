package blueprint

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/blueprints"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/model"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestProjectsGolden resolves every project in testdata/projects (each a
// folder with .esq files and, optionally, local blueprints) and compares the
// diagnostics, plus a summary of the resolved set when there are no errors,
// to testdata/projects/<name>.golden.
func TestProjectsGolden(t *testing.T) {
	dirs, _ := filepath.Glob("testdata/projects/*")
	var n int
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := resolveDir(t, dir)
			golden := dir + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run go test ./internal/blueprint -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("output differs from %s (run with -update to accept)\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
			}
		})
	}
	if n == 0 {
		t.Fatal("no test projects")
	}
}

func resolveDir(t *testing.T, dir string) string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*.esq"))
	var sources []model.Source
	srcs := map[string][]byte{}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, model.Source{Name: p, Src: src})
		srcs[p] = src
	}
	p, diags := model.Load(sources...)
	if diags.HasErrors() {
		var b strings.Builder
		diag.Render(&b, diags, func(n string) []byte { return srcs[n] })
		t.Fatalf("test project does not build:\n%s", b.String())
	}
	l := &Loader{Builtin: blueprints.FS, ProjectDir: dir}
	set, diags := Resolve(p, l)

	var b strings.Builder
	diag.Render(&b, diags, func(n string) []byte {
		if s, ok := srcs[n]; ok {
			return s
		}
		return l.Source(n)
	})
	if !diags.HasErrors() {
		b.WriteString(summary(p, set))
	}
	return b.String()
}

// summary describes the resolved set: order, options and annotations.
func summary(p *model.Project, set *Set) string {
	var b strings.Builder
	b.WriteString("order:")
	for _, bp := range set.Blueprints {
		fmt.Fprintf(&b, " %s(%s)", bp.Manifest.Name, bp.Location)
	}
	b.WriteString("\n")
	for _, bp := range set.Blueprints {
		fmt.Fprintf(&b, "options %s: %s\n", bp.Manifest.Name, sortedMap(set.Options[bp.Manifest.Name]))
	}
	ann := func(where string, a model.Ann) {
		names := make([]string, 0, len(a))
		for n := range a {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(&b, "@%s on %s: %s\n", n, where, sortedMap(a[n]))
		}
	}
	ann("app", p.App.Ann)
	for _, e := range p.Entities {
		ann(e.Raw, e.Ann)
		for _, f := range e.Fields {
			ann(e.Raw+"."+f.Raw, f.Ann)
		}
		for _, r := range e.Relations {
			ann(e.Raw+"."+r.Raw, r.Ann)
		}
	}
	return b.String()
}

func sortedMap(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%#v", k, m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func TestBuiltinBlueprintsAreValid(t *testing.T) {
	l := &Loader{Builtin: blueprints.FS}
	names := l.BuiltinNames()
	if strings.Join(names, " ") != "base postgres" {
		t.Fatalf("built-in blueprints = %v", names)
	}
	for _, name := range names {
		ref := &model.BlueprintRef{Ref: name, Source: model.SourceBuiltin}
		b, diags := l.Load(ref)
		if b == nil || len(diags) > 0 {
			t.Errorf("built-in %s: %v", name, diags)
		}
	}
}
