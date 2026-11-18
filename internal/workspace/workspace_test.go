package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/compose"
)

// folder is a project folder under test.
type folder struct {
	t   *testing.T
	dir string
}

func newProject(t *testing.T) *folder { return &folder{t: t, dir: t.TempDir()} }

func (p *folder) read(path string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(p.dir, path))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		p.t.Fatal(err)
	}
	return string(data), true
}

func (p *folder) write(path, content string) {
	full := filepath.Join(p.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

// gen plans and writes a generation, returning the plan as "change path"
// lines and Write's error.
func (p *folder) gen(force bool, files ...*compose.File) (string, error) {
	p.t.Helper()
	lock, err := ReadLock(p.dir)
	if err != nil {
		p.t.Fatal(err)
	}
	plan, err := MakePlan(p.dir, &compose.Result{Files: files}, lock)
	if err != nil {
		p.t.Fatal(err)
	}
	var lines []string
	for _, a := range plan.Actions {
		lines = append(lines, a.Change.String()+" "+a.Path)
	}
	err = Write(p.dir, plan, force, "test", []Blueprint{{Name: "core", Version: "0.1.0", Location: "built-in"}})
	return strings.Join(lines, "\n"), err
}

func file(path, content string) *compose.File {
	return &compose.File{Path: path, Content: []byte(content), Blueprint: "core", Template: path + ".tmpl"}
}

func once(path, content string) *compose.File {
	f := file(path, content)
	f.Once = true
	return f
}

func expect(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("plan:\n%s\nwant:\n%s", got, want)
	}
}

func TestFirstGenerationCreatesEverything(t *testing.T) {
	p := newProject(t)
	plan, err := p.gen(false, file("a.go", "A"), file("x/y/b.go", "B"), once("README.md", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "create README.md\ncreate a.go\ncreate x/y/b.go")
	if got, _ := p.read("x/y/b.go"); got != "B" {
		t.Fatalf("b.go = %q", got)
	}
	lock, _ := ReadLock(p.dir)
	if lock.Files["a.go"].Hash != Hash([]byte("A")) || !lock.Files["README.md"].Once || lock.Esquadro != "test" {
		t.Fatalf("lock = %+v", lock)
	}
	entries, _ := os.ReadDir(filepath.Join(p.dir, Dir))
	if len(entries) != 1 || entries[0].Name() != "lock.json" {
		t.Fatalf("esquadro/ has leftovers: %v", entries)
	}
}

func TestRegenerationUpdatesOnlyUntouchedFiles(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("a.go", "A1"), file("b.go", "B1"), once("README.md", "v1")); err != nil {
		t.Fatal(err)
	}
	plan, err := p.gen(false, file("a.go", "A2"), file("b.go", "B1"), once("README.md", "v2"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "update README.md\nupdate a.go\nunchanged b.go")
	if got, _ := p.read("a.go"); got != "A2" {
		t.Fatalf("a.go = %q", got)
	}
	if got, _ := p.read("README.md"); got != "v2" {
		t.Fatalf("a .once file the user never touched follows its template, got %q", got)
	}
}

func TestOnceFileTheUserChangedIsKept(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, once("README.md", "v1")); err != nil {
		t.Fatal(err)
	}
	p.write("README.md", "mine")
	plan, err := p.gen(false, once("README.md", "v2"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "keep README.md")
	if got, _ := p.read("README.md"); got != "mine" {
		t.Fatalf("a .once file the user changed is theirs, got %q", got)
	}
	// Changed back to what esquadro wrote, it follows the template again.
	p.write("README.md", "v1")
	if plan, err = p.gen(false, once("README.md", "v3")); err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "update README.md")
}

func TestEditedFileIsAConflictAndNothingIsWritten(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("a.go", "A1"), file("b.go", "B1")); err != nil {
		t.Fatal(err)
	}
	p.write("a.go", "A1 with my change")

	plan, err := p.gen(false, file("a.go", "A2"), file("b.go", "B2"))
	if !errors.Is(err, ErrConflicts) {
		t.Fatalf("err = %v", err)
	}
	expect(t, plan, "conflict a.go\nupdate b.go")
	if got, _ := p.read("a.go"); got != "A1 with my change" {
		t.Fatalf("a.go = %q", got)
	}
	if got, _ := p.read("b.go"); got != "B1" {
		t.Fatalf("with a conflict nothing is written, but b.go = %q", got)
	}

	if _, err := p.gen(true, file("a.go", "A2"), file("b.go", "B2")); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.read("a.go"); got != "A2" {
		t.Fatalf("forced a.go = %q", got)
	}
}

func TestEditedFileMatchingTheNewOutputIsAdopted(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("a.go", "A1")); err != nil {
		t.Fatal(err)
	}
	p.write("a.go", "A2")
	plan, err := p.gen(false, file("a.go", "A2"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "unchanged a.go")
	if lock, _ := ReadLock(p.dir); lock.Files["a.go"].Hash != Hash([]byte("A2")) {
		t.Fatal("the lock must record the adopted content")
	}
}

func TestFileNotGeneratedByEsquadroIsAConflict(t *testing.T) {
	p := newProject(t)
	p.write("a.go", "mine")
	plan, err := p.gen(false, file("a.go", "generated"))
	if !errors.Is(err, ErrConflicts) {
		t.Fatalf("err = %v", err)
	}
	expect(t, plan, "conflict a.go")
}

func TestFilesNoLongerGeneratedAreDeleted(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("keep.go", "K"), file("gone/deep/x.go", "X"), file("gone/y.go", "Y"), once("notes.md", "n")); err != nil {
		t.Fatal(err)
	}
	p.write("gone/mine.txt", "user file")
	plan, err := p.gen(false, file("keep.go", "K"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "delete gone/deep/x.go\ndelete gone/y.go\nunchanged keep.go")
	if _, ok := p.read("gone/deep/x.go"); ok {
		t.Fatal("x.go not deleted")
	}
	if _, err := os.Stat(filepath.Join(p.dir, "gone/deep")); !os.IsNotExist(err) {
		t.Fatal("an emptied folder must be removed")
	}
	if _, ok := p.read("gone/mine.txt"); !ok {
		t.Fatal("a folder with the user's files must stay")
	}
	if got, ok := p.read("notes.md"); !ok || got != "n" {
		t.Fatal("a .once file no longer generated stays the user's")
	}
	if lock, _ := ReadLock(p.dir); len(lock.Files) != 1 {
		t.Fatalf("lock files = %v", lock.Files)
	}
}

func TestEditedFileNoLongerGeneratedIsAConflict(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("a.go", "A")); err != nil {
		t.Fatal(err)
	}
	p.write("a.go", "A edited")
	plan, err := p.gen(false)
	if !errors.Is(err, ErrConflicts) {
		t.Fatalf("err = %v", err)
	}
	expect(t, plan, "conflict a.go")
	if _, err := p.gen(true); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.read("a.go"); ok {
		t.Fatal("forced generation must delete it")
	}
}

func TestDeletedOnceFileIsNotRecreated(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, once("README.md", "hello")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(p.dir, "README.md"))
	plan, err := p.gen(false, once("README.md", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "keep README.md")
	if _, ok := p.read("README.md"); ok {
		t.Fatal("README.md was recreated")
	}
}

func TestOnceFileTheUserAlreadyHadIsKept(t *testing.T) {
	p := newProject(t)
	p.write("README.md", "my readme")
	plan, err := p.gen(false, once("README.md", "generated readme"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "keep README.md")
	if got, _ := p.read("README.md"); got != "my readme" {
		t.Fatalf("README.md = %q", got)
	}
}

func TestAGeneratedFileThatBecomesTheUsersIsReplacedWhenUntouched(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("routes.go", "generated")); err != nil {
		t.Fatal(err)
	}
	plan, err := p.gen(false, once("routes.go", "yours"))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "update routes.go")
	if got, _ := p.read("routes.go"); got != "yours" {
		t.Fatalf("routes.go = %q", got)
	}
	// From now on it is the user's.
	p.write("routes.go", "mine")
	if plan, err = p.gen(false, once("routes.go", "yours, again")); err != nil {
		t.Fatal(err)
	}
	expect(t, plan, "keep routes.go")

	// Edited while generated: a conflict, not overwritten.
	q := newProject(t)
	if _, err := q.gen(false, file("routes.go", "generated")); err != nil {
		t.Fatal(err)
	}
	q.write("routes.go", "edited")
	if _, err := q.gen(false, once("routes.go", "yours")); err == nil {
		t.Fatal("an edited generated file was replaced")
	}
	if got, _ := q.read("routes.go"); got != "edited" {
		t.Fatalf("routes.go = %q", got)
	}
}

func TestUpdateKeepsFileMode(t *testing.T) {
	p := newProject(t)
	if _, err := p.gen(false, file("run.sh", "v1")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(p.dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := p.gen(false, file("run.sh", "v2")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(p.dir, "run.sh"))
	// Windows has no Unix permissions to keep.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestDiff(t *testing.T) {
	a := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	b := "one\ntwo\nTHREE\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\n"
	want := `--- a/f.txt
+++ b/f.txt
@@ -1,6 +1,6 @@
 one
 two
-three
+THREE
 four
 five
 six
@@ -8,3 +8,4 @@
 eight
 nine
 ten
+eleven
`
	if got := Diff("f.txt", []byte(a), []byte(b)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if Diff("f", []byte("x"), []byte("x")) != "" {
		t.Fatal("equal inputs must give no diff")
	}
	if got := Diff("f", nil, []byte("new\n")); got != "--- a/f\n+++ b/f\n@@ -0,0 +1,1 @@\n+new\n" {
		t.Fatalf("diff of a new file:\n%s", got)
	}
	if got := Diff("f", []byte("x"), []byte("y")); !strings.Contains(got, "-x\n\\ No newline at end of file\n+y\n\\ No newline at end of file\n") {
		t.Fatalf("missing newline marker:\n%s", got)
	}
}
