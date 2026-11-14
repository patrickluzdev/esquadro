package hook

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/compose"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
	"github.com/patrickluzdev/esquadro/internal/workspace"
)

func TestMatch(t *testing.T) {
	yes := [][2]string{
		{"internal/*/sqlgen/**", "internal/catalog/sqlgen/db.go"},
		{"internal/*/sqlgen/**", "internal/catalog/sqlgen/deep/x.go"},
		{"**/*.sql", "queries.sql"},
		{"**/*.sql", "db/x/queries.sql"},
		{"gen/**", "gen/out.txt"},
		{"a.txt", "a.txt"},
		{"**", "anything/at/all"},
	}
	no := [][2]string{
		{"internal/*/sqlgen/**", "internal/catalog/db.go"},
		{"internal/*/sqlgen/**", "internal/a/b/sqlgen/x.go"},
		{"**/*.sql", "queries.go"},
		{"gen/**", "general/x"},
		{"a.txt", "b/a.txt"},
	}
	for _, c := range yes {
		if !Match(c[0], c[1]) {
			t.Errorf("Match(%q, %q) = false", c[0], c[1])
		}
	}
	for _, c := range no {
		if Match(c[0], c[1]) {
			t.Errorf("Match(%q, %q) = true", c[0], c[1])
		}
	}
}

// fixture is a project with a local blueprint "demo" whose hooks are shell
// scripts.
type fixture struct {
	t   *testing.T
	dir string
	out strings.Builder
}

func newFixture(t *testing.T, manifestHooks string, scripts map[string]string) *fixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hook scripts are POSIX shell")
	}
	f := &fixture{t: t, dir: t.TempDir()}
	f.write("shop.esq", "package catalog\n\napp shop {\n  module \"github.com/example/shop\"\n  blueprint \"./bp/demo\"\n}\n\nentity Product {\n  name string\n}\n", 0o644)
	f.write("bp/demo/blueprint.esq", "blueprint demo {\n  version \"0.1.0\"\n"+manifestHooks+"}\n", 0o644)
	f.write("bp/demo/templates/catalog/{{.Entity.Snake}}.txt.tmpl", "{{.Entity.Pascal}}\n", 0o644)
	for name, body := range scripts {
		f.write("bp/demo/hooks/"+name, "#!/bin/sh\n"+body+"\n", 0o755)
	}
	return f
}

func (f *fixture) write(path, content string, mode os.FileMode) {
	full := filepath.Join(f.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) load() (*project.Project, *blueprint.Set, *compose.Result) {
	f.t.Helper()
	src, _ := os.ReadFile(filepath.Join(f.dir, "shop.esq"))
	p, diags := project.Load(project.Source{Name: "shop.esq", Src: src})
	if diags.HasErrors() {
		f.t.Fatal(diags)
	}
	l := &blueprint.Loader{ProjectDir: f.dir}
	set, diags := blueprint.Resolve(p, l)
	if diags.HasErrors() {
		var b strings.Builder
		diag.Render(&b, diags, l.Source)
		f.t.Fatal(b.String())
	}
	res, _, diags := compose.Compose(p, set)
	if diags.HasErrors() {
		f.t.Fatal(diags)
	}
	return p, set, res
}

func (f *fixture) runner() *Runner { return &Runner{ProjectDir: f.dir, Out: &f.out} }

// generate writes the composed files and runs the post-gen hooks, as gen does.
func (f *fixture) generate(force bool) error {
	f.t.Helper()
	p, set, res := f.load()
	r := f.runner()
	if err := r.PreGen(context.Background(), p, set, res, false); err != nil {
		return err
	}
	lock, err := workspace.ReadLock(f.dir)
	if err != nil {
		f.t.Fatal(err)
	}
	plan, err := workspace.MakePlan(f.dir, res, lock)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := workspace.Write(f.dir, plan, force, "test", nil); err != nil {
		return err
	}
	return r.PostGen(context.Background(), p, set, res)
}

func TestPreGenAddsFilesAndSeesTheInput(t *testing.T) {
	f := newFixture(t, `  hook pre-gen "hooks/pre"`+"\n", map[string]string{
		"pre": `input=$(cat)
event=$(echo "$input" | grep -o '"event":"[a-z-]*"' | cut -d'"' -f4)
entity=$(echo "$input" | grep -o '"pascal":"Product"' | head -1 | cut -d'"' -f4)
echo "debug line" >&2
printf '{"messages":[{"level":"warning","text":"heads up"}],"files":[{"path":"from-hook.txt","content":"%s %s"}]}' "$event" "$entity"`,
	})
	p, set, res := f.load()
	if err := f.runner().PreGen(context.Background(), p, set, res, false); err != nil {
		t.Fatal(err)
	}
	got := res.File("from-hook.txt")
	if got == nil || string(got.Content) != "pre-gen Product" || got.Blueprint != "demo" || got.Template != "hook hooks/pre" {
		t.Fatalf("hook file = %+v", got)
	}
	out := f.out.String()
	if !strings.Contains(out, "demo pre-gen hooks/pre: warning: heads up") || !strings.Contains(out, "demo pre-gen hooks/pre: debug line") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestPreGenCannotReplaceATemplateFile(t *testing.T) {
	f := newFixture(t, `  hook pre-gen "hooks/pre"`+"\n", map[string]string{
		"pre": `cat >/dev/null; printf '{"files":[{"path":"catalog/product.txt","content":"x"}]}'`,
	})
	p, set, res := f.load()
	err := f.runner().PreGen(context.Background(), p, set, res, false)
	if err == nil || !strings.Contains(err.Error(), "catalog/product.txt, which blueprint demo already generates") {
		t.Fatalf("err = %v", err)
	}
}

func TestPostGenWritesAreTrackedAndProtected(t *testing.T) {
	f := newFixture(t, `  hook post-gen "hooks/post" writes "gen/**"`+"\n", map[string]string{
		"post": `cat >/dev/null; mkdir -p gen; echo "written by hook" > gen/out.txt`,
	})
	if err := f.generate(false); err != nil {
		t.Fatal(err)
	}
	lock, _ := workspace.ReadLock(f.dir)
	entry, ok := lock.Files["gen/out.txt"]
	if !ok || entry.Hook != "demo post-gen hooks/post" || entry.Blueprint != "demo" {
		t.Fatalf("lock entry = %+v (%v)", entry, ok)
	}
	if _, ok := lock.Files["catalog/product.txt"]; !ok {
		t.Fatal("template files must stay in the lock")
	}

	// Running again is fine: the hook rewrites its own file.
	if err := f.generate(false); err != nil {
		t.Fatal(err)
	}

	// An edit to a file the hook wrote is protected like any generated file.
	f.write("gen/out.txt", "my edit\n", 0o644)
	if err := f.generate(false); err != workspace.ErrConflicts {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(f.dir, "gen/out.txt")); string(data) != "my edit\n" {
		t.Fatal("the edit was lost")
	}
	if err := f.generate(true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(f.dir, "gen/out.txt")); string(data) != "written by hook\n" {
		t.Fatalf("forced: %q", data)
	}
}

func TestPostGenMayNotWriteOutsideItsGlobs(t *testing.T) {
	f := newFixture(t, `  hook post-gen "hooks/post" writes "gen/**"`+"\n", map[string]string{
		"post": `cat >/dev/null; echo x > outside.txt`,
	})
	err := f.generate(false)
	if err == nil || !strings.Contains(err.Error(), "changed files outside what it declares in writes and edits (gen/**): outside.txt") {
		t.Fatalf("err = %v", err)
	}
}

func TestHookFailures(t *testing.T) {
	cases := map[string]struct{ script, want string }{
		"exit code":   {`cat >/dev/null; echo boom >&2; exit 3`, "hook demo pre-gen hooks/pre failed: exit status 3"},
		"bad output":  {`cat >/dev/null; echo "not json"`, "its output is not the expected JSON"},
		"no exec bit": {"", "is not executable (run: chmod +x"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, `  hook pre-gen "hooks/pre"`+"\n", map[string]string{"pre": c.script})
			if name == "no exec bit" {
				os.Chmod(filepath.Join(f.dir, "bp/demo/hooks/pre"), 0o644)
			}
			p, set, res := f.load()
			err := f.runner().PreGen(context.Background(), p, set, res, false)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestBuiltinHookRunsInProcess(t *testing.T) {
	f := newFixture(t, "", nil)
	p, set, res := f.load()
	bp := set.Blueprints[0]
	bp.Location = "built-in"
	bp.Manifest.Hooks = []*blueprint.Hook{{Event: "pre-gen", Path: "make-readme"}}
	var seen Input
	r := f.runner()
	r.Builtin = map[string]Func{
		Key("demo", "make-readme"): func(_ context.Context, in Input) (Output, error) {
			seen = in
			return Output{Files: []File{{Path: "README.md", Content: "# " + in.Project.App.Pascal}}}, nil
		},
	}
	if err := r.PreGen(context.Background(), p, set, res, true); err != nil {
		t.Fatal(err)
	}
	if got := res.File("README.md"); got == nil || string(got.Content) != "# Shop" {
		t.Fatalf("README = %+v", got)
	}
	if !seen.DryRun || seen.Event != "pre-gen" || len(seen.Files) != 1 || seen.Files[0] != "catalog/product.txt" {
		t.Fatalf("input = %+v", seen)
	}
}

func TestCheckOverlaps(t *testing.T) {
	f := newFixture(t, `  hook post-gen "hooks/post" writes "catalog/**"`+"\n", map[string]string{"post": "cat >/dev/null"})
	_, set, res := f.load()
	diags := CheckOverlaps(set, res)
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "may change catalog/product.txt, which blueprint demo also generates") {
		t.Fatalf("diags = %v", diags)
	}
}

func TestPostGenEditsStayTheUsers(t *testing.T) {
	f := newFixture(t, `  hook post-gen "hooks/post" edits "notes.txt"`+"\n", map[string]string{
		"post": `cat >/dev/null; echo "line from hook" >> notes.txt`,
	})
	if err := f.generate(false); err != nil {
		t.Fatal(err)
	}
	lock, _ := workspace.ReadLock(f.dir)
	if _, tracked := lock.Files["notes.txt"]; tracked {
		t.Fatal("files a hook edits must not be tracked")
	}
	f.write("notes.txt", "my own notes\n", 0o644)
	if err := f.generate(false); err != nil {
		t.Fatalf("an edit to a file the hook edits must never conflict: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(f.dir, "notes.txt")); string(data) != "my own notes\nline from hook\n" {
		t.Fatalf("notes.txt = %q", data)
	}
}
