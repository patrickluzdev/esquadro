package compose

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/model"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestCasesGolden composes every project in testdata/cases. Each case is a
// folder with .esq files and a blueprints/ folder, used as the built-in
// blueprints. The output (diagnostics, then every generated file) is
// compared to testdata/cases/<name>.golden.
func TestCasesGolden(t *testing.T) {
	dirs, _ := filepath.Glob("testdata/cases/*")
	var n int
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := composeDir(t, dir)
			golden := dir + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run go test ./internal/compose -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("output differs from %s (run with -update to accept)\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
			}
		})
	}
	if n == 0 {
		t.Fatal("no test cases")
	}
}

func composeDir(t *testing.T, dir string) string {
	t.Helper()
	p, set := load(t, dir)
	res, srcs, diags := Compose(p, set)

	var b strings.Builder
	diag.Render(&b, diags, srcs.Get)
	if len(diags) > 0 {
		b.WriteString("\n")
	}
	for _, f := range res.Files {
		fmt.Fprintf(&b, "=== %s  [%s: %s]", f.Path, f.Blueprint, f.Template)
		if f.Once {
			b.WriteString(" once")
		}
		if len(f.Replaced) > 0 {
			fmt.Fprintf(&b, " replaced=%s", strings.Join(f.Replaced, ","))
		}
		for _, s := range f.Slots {
			fmt.Fprintf(&b, " slot:%s<-%s", s.Slot, strings.Join(s.Contributors, ","))
		}
		b.WriteString("\n")
		b.Write(f.Content)
		if len(f.Content) > 0 && f.Content[len(f.Content)-1] != '\n' {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func load(t *testing.T, dir string) (*model.Project, *blueprint.Set) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*.esq"))
	var sources []model.Source
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, model.Source{Name: filepath.Base(path), Src: src})
	}
	p, diags := model.Load(sources...)
	if diags.HasErrors() {
		t.Fatalf("test project does not build: %v", diags)
	}
	l := &blueprint.Loader{Builtin: os.DirFS(filepath.Join(dir, "blueprints")), ProjectDir: dir}
	set, diags := blueprint.Resolve(p, l)
	if diags.HasErrors() {
		var b strings.Builder
		diag.Render(&b, diags, l.Source)
		t.Fatalf("test blueprints do not resolve:\n%s", b.String())
	}
	return p, set
}

func TestCleanTarget(t *testing.T) {
	ok := map[string]string{"a.go": "a.go", "x/./y.go": "x/y.go", "x/../y.go": "y.go"}
	for in, want := range ok {
		if got, valid := cleanTarget(in); !valid || got != want {
			t.Errorf("cleanTarget(%q) = %q, %v", in, got, valid)
		}
	}
	for _, in := range []string{"", "/abs", "../x", "x/../../y", "a//b", "dir/", `a\b`, ".esquadro/lock.json", ".esquadro", "."} {
		if _, valid := cleanTarget(in); valid {
			t.Errorf("cleanTarget(%q) should be rejected", in)
		}
	}
}
