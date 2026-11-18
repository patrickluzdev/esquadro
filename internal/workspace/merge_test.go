package workspace

import (
	"strings"
	"testing"
)

func lines(s ...string) []byte { return []byte(strings.Join(s, "\n") + "\n") }

func TestMerge3TakesEachSidesChanges(t *testing.T) {
	base := lines("a", "b", "c", "d", "e")
	ours := lines("a", "B mine", "c", "d", "e")       // the user changed b
	theirs := lines("a", "b", "c", "d", "e", "f new") // esquadro added f
	got, conflicts, ok := Merge3(base, ours, theirs)
	if !ok || conflicts != 0 {
		t.Fatalf("ok %v, %d conflicts", ok, conflicts)
	}
	if want := string(lines("a", "B mine", "c", "d", "e", "f new")); string(got) != want {
		t.Fatalf("merged:\n%s\nwant:\n%s", got, want)
	}
}

func TestMerge3SameChangeOnBothSidesOnce(t *testing.T) {
	base := lines("a", "b", "c")
	both := lines("a", "B", "c")
	got, conflicts, _ := Merge3(base, both, both)
	if conflicts != 0 || string(got) != string(both) {
		t.Fatalf("%d conflicts:\n%s", conflicts, got)
	}
}

func TestMerge3MarksTheConflicts(t *testing.T) {
	base := lines("a", "b", "c")
	ours := lines("a", "mine", "c")
	theirs := lines("a", "esquadro's", "c")
	got, conflicts, ok := Merge3(base, ours, theirs)
	if !ok || conflicts != 1 {
		t.Fatalf("ok %v, %d conflicts", ok, conflicts)
	}
	want := string(lines("a", "<<<<<<< yours", "mine", "=======", "esquadro's", ">>>>>>> esquadro", "c"))
	if string(got) != want {
		t.Fatalf("merged:\n%s\nwant:\n%s", got, want)
	}
	if !hasConflictMarkers(got) {
		t.Fatal("the markers are not found")
	}
}

func TestMerge3KeepsTheUsersDeletionsAndAdditions(t *testing.T) {
	base := lines("import a", "import b", "", "func f() {}", "", "func g() {}", "", "// end")
	// The user removed import b and added a function at the end.
	ours := lines("import a", "", "func f() {}", "", "func g() {}", "", "// end", "func mine() {}")
	// esquadro changed g.
	theirs := lines("import a", "import b", "", "func f() {}", "", "func g() { new() }", "", "// end")
	got, conflicts, _ := Merge3(base, ours, theirs)
	if conflicts != 0 {
		t.Fatalf("%d conflicts:\n%s", conflicts, got)
	}
	if want := string(lines("import a", "", "func f() {}", "", "func g() { new() }", "", "// end", "func mine() {}")); string(got) != want {
		t.Fatalf("merged:\n%s\nwant:\n%s", got, want)
	}
}

func TestMerge3ChangesTouchingTheSameLinesConflict(t *testing.T) {
	// Removing a line next to where esquadro adds one is a conflict, as
	// git has it: which order the user wants is theirs to say.
	base := lines("import a", "import b", "")
	ours := lines("import a", "")
	theirs := lines("import a", "import b", "import c", "")
	if _, conflicts, _ := Merge3(base, ours, theirs); conflicts != 1 {
		t.Fatalf("%d conflicts", conflicts)
	}
}
