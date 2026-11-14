package main

import (
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "esquadro ") {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"nope"}, &out, &errOut); code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), `comando desconhecido "nope"`) {
		t.Fatalf("unexpected stderr %q", errOut.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut strings.Builder
	if code := run(nil, &out, &errOut); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "version") {
		t.Fatalf("help does not list commands: %q", out.String())
	}
}
