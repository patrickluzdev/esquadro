package dsl

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestTheEditorKnowsEveryKeyword keeps the VS Code extension in step with
// the language: its grammar colors every top-level keyword, and its README,
// the extension's page on the marketplaces, names them. The README names no
// version either: a release stamps it.
func TestTheEditorKnowsEveryKeyword(t *testing.T) {
	readme, err := os.ReadFile("../../editors/vscode/README.md")
	if err != nil {
		t.Fatal(err)
	}
	grammar, err := os.ReadFile("../../editors/vscode/syntaxes/esquadro.tmLanguage.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kw := range topKeywords {
		if !strings.Contains(string(grammar), kw) {
			t.Errorf("the grammar does not color %q", kw)
		}
		if kw != "blueprint" && !strings.Contains(string(readme), "`"+kw+"`") {
			t.Errorf("editors/vscode/README.md does not name the keyword %q", kw)
		}
	}
	if v := regexp.MustCompile(`\b\d+\.\d+\.\d+\b`).FindString(string(readme)); v != "" {
		t.Errorf("editors/vscode/README.md names a version, %s: it would go stale", v)
	}
}
