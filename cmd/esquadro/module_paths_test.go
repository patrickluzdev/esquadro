package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
)

// TestEveryFileFitsInAModuleZip checks every file of the repository against
// the rules the go command applies when it zips a module version: one file
// it refuses (a quote or a backtick in a test fixture's name) breaks
// go install for that version, for good.
func TestEveryFileFitsInAModuleZip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		t.Skip("not in a git checkout")
	}
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	for f := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if _, err := os.Stat(filepath.Join(strings.TrimSpace(string(root)), f)); err != nil {
			continue // deleted in the working tree
		}
		if err := module.CheckFilePath(f); err != nil {
			t.Errorf("%v", err)
		}
	}
}
