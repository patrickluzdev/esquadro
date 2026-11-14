package dsl

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestParseGolden parses every testdata/parse/*.esq file, which must have no
// errors, and compares the tree dump to the .golden file next to it.
func TestParseGolden(t *testing.T) {
	runGolden(t, "testdata/parse", func(t *testing.T, name string, src []byte) string {
		f, diags := Parse(name, src)
		if len(diags) > 0 {
			var b strings.Builder
			diag.Render(&b, diags, func(string) []byte { return src })
			t.Fatalf("unexpected diagnostics:\n%s", b.String())
		}
		return Dump(f)
	})
}

// TestErrorsGolden parses every testdata/errors/*.esq file and compares the
// rendered diagnostics to the .golden file next to it.
func TestErrorsGolden(t *testing.T) {
	runGolden(t, "testdata/errors", func(t *testing.T, name string, src []byte) string {
		_, diags := Parse(name, src)
		if !diags.HasErrors() {
			t.Fatal("expected errors, got none")
		}
		var b strings.Builder
		diag.Render(&b, diags, func(string) []byte { return src })
		return b.String()
	})
}

func runGolden(t *testing.T, dir string, render func(*testing.T, string, []byte) string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.esq"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files in %s: %v", dir, err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(strings.TrimSuffix(name, ".esq"), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := render(t, name, src)
			golden := strings.TrimSuffix(path, ".esq") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run go test ./internal/dsl -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("output differs from %s (run with -update to accept)\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
			}
		})
	}
}

func TestParseIsDeterministicAndKeepsGoing(t *testing.T) {
	src := []byte("entity A {\n  x\n}\nentity B {\n  y\n}\nentity C {\n  z int\n}\n")
	f, diags := Parse("t.esq", src)
	if len(diags) != 2 {
		t.Fatalf("want 2 errors (one per broken entity), got %d: %v", len(diags), diags)
	}
	if len(f.Decls) != 3 {
		t.Fatalf("want all 3 entities in the tree, got %d", len(f.Decls))
	}
	if c := f.Decls[2].(*EntityDecl); len(c.Fields) != 1 || c.Fields[0].Name.Name != "z" {
		t.Fatalf("entity C not parsed correctly: %+v", c)
	}
}

func TestLexerPositionsCountRunes(t *testing.T) {
	var diags diag.List
	toks, _ := lex("t.esq", []byte("naïve > Größe\n"), &diags)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	want := []struct {
		kind     Kind
		text     string
		col, end int
	}{
		{Name, "naïve", 1, 6},
		{Gt, ">", 7, 8},
		{Name, "Größe", 9, 14},
		{Newline, "\n", 14, 1},
		{EOF, "", 1, 1},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, w := range want {
		tk := toks[i]
		if tk.Kind != w.kind || tk.Text != w.text || tk.Pos.Col != w.col || tk.End.Col != w.end {
			t.Errorf("token %d = %v %q %d-%d, want %v %q %d-%d", i, tk.Kind, tk.Text, tk.Pos.Col, tk.End.Col, w.kind, w.text, w.col, w.end)
		}
	}
}

func TestLexerRelationSignsAndHyphenatedNames(t *testing.T) {
	var diags diag.List
	toks, _ := lex("t.esq", []byte("admin-htmx a > b c <> d e - f g < H.h -5 x-1"), &diags)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var got []string
	for _, tk := range toks {
		if tk.Kind != EOF {
			got = append(got, tk.Kind.String()+":"+tk.Text)
		}
	}
	want := `name:admin-htmx name:a ">":> name:b name:c "<>":<> name:d name:e "-":- name:f name:g "<":< name:H ".":. name:h number:-5 name:x-1`
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}

func TestParseEmptyFile(t *testing.T) {
	f, diags := Parse("empty.esq", nil)
	if len(diags) != 0 || len(f.Decls) != 0 {
		t.Fatalf("empty file should parse cleanly, got %v", diags)
	}
}
