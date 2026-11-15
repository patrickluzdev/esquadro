package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeTools puts scripts that act as the given tools alone on PATH.
func fakeTools(t *testing.T, tools map[string]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the tools are POSIX shell scripts")
	}
	bin := t.TempDir()
	for name, script := range tools {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func TestDoctorOutsideAProject(t *testing.T) {
	fakeTools(t, map[string]string{
		"go":     "echo go1.27.9",
		"git":    "echo git version 2.40.0",
		"docker": "exit 1",
	})
	code, out, _ := esq(t, "doctor", t.TempDir())
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{
		"no project here",
		"  ok    go 1.27.9\n",
		"  ok    git 2.40.0\n",
		"  warn  task is not installed: the project's tasks",
		"        install it: https://taskfile.dev/installation/\n",
		"  warn  docker is installed but not running",
		"Ready, with 2 warnings.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDoctorFailsOnWhatTheProjectNeeds(t *testing.T) {
	fakeTools(t, map[string]string{
		"go":   "echo go1.26.1",
		"task": "echo 3.54.0",
	})
	dir, _ := smallProject(t, "app,compose")
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/shop\n\ngo 1.27\n"), 0o644)
	code, out, _ := esq(t, "doctor", dir)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{
		"  fail  go 1.26.1 is older than 1.27, which go.mod asks for\n",
		"  warn  git is not installed",
		"  ok    task 3.54.0\n",
		"  fail  docker is not installed: compose",
		"  ok    the project checks\n",
		"  fail  the generated code is not what the model generates\n        see what would change: esquadro gen -dry-run",
		"3 problems to fix.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDoctorOnAProjectWithErrors(t *testing.T) {
	fakeTools(t, map[string]string{"go": "echo go1.27.0"})
	dir, appEsq := smallProject(t, "")
	os.WriteFile(appEsq, []byte("app shop {\n  blueprint nope\n}\n"), 0o644)
	if code, out, _ := esq(t, "doctor", dir); code != 1 || !strings.Contains(out, "  fail  the project has errors\n        see them with: esquadro check") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestOlderGo(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		older bool
	}{
		{"1.26.9", "1.27", true}, {"1.27.0", "1.27", false}, {"1.28", "1.27", false}, {"2.0", "1.27", false},
	} {
		if got := olderGo(tc.a, tc.b); got != tc.older {
			t.Errorf("olderGo(%s, %s) = %v", tc.a, tc.b, got)
		}
	}
}
