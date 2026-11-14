package dsl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

// FuzzParse checks that the parser never panics or hangs (2s limit) on
// arbitrary input, and that every diagnostic points inside the file.
// Run longer with: go test ./internal/dsl -fuzz FuzzParse -fuzztime 1m
func FuzzParse(f *testing.F) {
	for _, dir := range []string{"testdata/parse", "testdata/errors"} {
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
		var diags diag.List
		done := make(chan struct{})
		go func() { _, diags = Parse("fuzz.esq", src); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("parser did not finish within 2s on %q", src)
		}
		for _, d := range diags {
			if d.Pos.Line < 1 || d.Pos.Col < 1 || d.Pos.Offset > len(src) {
				t.Fatalf("diagnostic outside the file: %+v", d)
			}
		}
	})
}
