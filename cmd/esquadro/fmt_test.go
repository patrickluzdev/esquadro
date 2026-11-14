package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const messy = "app shop {\n    module \"github.com/example/shop\"\n  blueprint platform\n}\nentity Product {\n  name string required\n  description text\n}\n"

const tidy = "app shop {\n  module \"github.com/example/shop\"\n  blueprint platform\n}\n\nentity Product {\n  name        string required\n  description text\n}\n"

func fmtCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := run(append([]string{"fmt"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestFmtPrints(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", messy)

	code, out, errOut := fmtCmd(t)
	if code != 0 || out != tidy {
		t.Fatalf("exit %d, stderr %q\ngot:\n%s\nwant:\n%s", code, errOut, out, tidy)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "shop.esq")); string(got) != messy {
		t.Fatal("fmt without -w must not change the file")
	}
}

func TestFmtWrite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", messy)
	if err := os.Chmod(filepath.Join(dir, "shop.esq"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, out, errOut := fmtCmd(t, "-w"); code != 0 || out != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "shop.esq"))
	if string(got) != tidy {
		t.Fatalf("file not formatted:\n%s", got)
	}
	if info, _ := os.Stat(filepath.Join(dir, "shop.esq")); info.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestFmtCheck(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", messy)

	code, _, errOut := fmtCmd(t, "-check")
	if code != 1 || !strings.Contains(errOut, "shop.esq is not formatted") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	writeFile(t, dir, "shop.esq", tidy)
	if code, _, errOut := fmtCmd(t, "-check"); code != 0 {
		t.Fatalf("formatted file: exit %d, stderr %q", code, errOut)
	}
}

func TestFmtSyntaxError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "shop.esq", "entity Product {\n  name\n}\n")

	code, out, errOut := fmtCmd(t, "-w")
	if code != 1 || out != "" || !strings.Contains(errOut, "shop.esq:2:7: error:") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "shop.esq")); string(got) != "entity Product {\n  name\n}\n" {
		t.Fatal("a file with errors must not be changed")
	}
}

func TestFmtFlags(t *testing.T) {
	if code, _, _ := fmtCmd(t, "-w", "-check"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestFmtDirectoryAndFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "a.esq", messy)
	writeFile(t, dir, "b.esq", tidy)

	code, _, errOut := fmtCmd(t, "-check")
	if code != 1 || !strings.Contains(errOut, "a.esq is not formatted") || strings.Contains(errOut, "b.esq") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := fmtCmd(t, "-w", "a.esq"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := fmtCmd(t, "-check", "."); code != 0 {
		t.Fatalf("after -w: exit %d, stderr %q", code, errOut)
	}
}

func TestFmtKeepsGoingAfterAnError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "a.esq", "entity Product {\n  name\n}\n")
	writeFile(t, dir, "b.esq", messy)

	code, _, errOut := fmtCmd(t, "-w")
	if code != 1 || !strings.Contains(errOut, "a.esq:2:7: error:") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "b.esq")); string(got) != tidy {
		t.Fatal("b.esq should still be formatted when a.esq has errors")
	}
}
