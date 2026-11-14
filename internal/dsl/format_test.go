package dsl

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

// TestFormatGolden formats every testdata/format/*.esq file and compares the
// result to the .golden file next to it.
func TestFormatGolden(t *testing.T) {
	runGolden(t, "testdata/format", func(t *testing.T, name string, src []byte) string {
		f, diags := Parse(name, src)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		return string(Format(f))
	})
}

// TestFormatProperties checks, for every valid test file, that formatting
// produces a valid file, is idempotent and does not change the meaning.
func TestFormatProperties(t *testing.T) {
	var files []string
	for _, dir := range []string{"testdata/parse", "testdata/format"} {
		m, _ := filepath.Glob(filepath.Join(dir, "*.esq"))
		files = append(files, m...)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if msg := checkFormat(src); msg != "" {
				t.Fatal(msg)
			}
		})
	}
}

// checkFormat returns a description of the first broken property, or "" when
// src is invalid (nothing to check) or every property holds.
func checkFormat(src []byte) string {
	f1, diags := Parse("a.esq", src)
	if diags.HasErrors() {
		return ""
	}
	out1 := Format(f1)
	f2, diags := Parse("b.esq", out1)
	if diags.HasErrors() {
		var b strings.Builder
		diag.Render(&b, diags, func(string) []byte { return out1 })
		return "formatted output does not parse:\n" + b.String() + "\n--- output ---\n" + string(out1)
	}
	if out2 := Format(f2); string(out2) != string(out1) {
		return "formatting is not idempotent:\n--- first ---\n" + string(out1) + "\n--- second ---\n" + string(out2)
	}
	if a, b := meaning(f1), meaning(f2); a != b {
		return "formatting changed the meaning:\n--- before ---\n" + a + "\n--- after ---\n" + b
	}
	return ""
}

var positions = regexp.MustCompile(` @\d+:\d+-\d+:\d+`)

// meaning is the tree dump without positions, with comments compared as a
// sorted list (formatting may move a comment, never drop or change it).
func meaning(f *File) string {
	var tree, comments []string
	for l := range strings.SplitSeq(positions.ReplaceAllString(Dump(f), ""), "\n") {
		if strings.HasPrefix(l, "comment ") {
			comments = append(comments, l)
		} else {
			tree = append(tree, l)
		}
	}
	sort.Strings(comments)
	return strings.Join(tree, "\n") + "\n" + strings.Join(comments, "\n")
}

// FuzzFormat checks the formatting properties on arbitrary valid input.
// Run longer with: go test ./internal/dsl -run '^$' -fuzz FuzzFormat -fuzztime 1m
func FuzzFormat(f *testing.F) {
	for _, dir := range []string{"testdata/parse", "testdata/format"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.esq"))
		for _, path := range files {
			src, err := os.ReadFile(path)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(src)
		}
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		if msg := checkFormat(src); msg != "" {
			t.Fatalf("%s\n--- input ---\n%q", msg, src)
		}
	})
}
