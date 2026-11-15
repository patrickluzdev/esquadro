package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbiddenNames are the generic role names the generated code must never
// use for files (the code-organization rule CODE-11 of the style esquadro
// generates).
var forbiddenNames = regexp.MustCompile(`(?i)(service|manager|repository|repo|dto|dao|controller|handler|helper|util|common|misc|base|impl|data|info|processor|model)`)

// TestPlatformGeneratesAWorkingProject generates a two-module project with the
// platform blueprint and checks that it builds, passes go vet and gopls, and
// names nothing by a technical role. It needs the go command; it does not
// need the network, since the project uses no module outside the standard
// library.
func TestPlatformGeneratesAWorkingProject(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a generated project")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is not on PATH")
	}
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("esquadro/app.esq", "app shop {\n  module \"example.com/shop\"\n  blueprint platform\n}\n")
	write("esquadro/catalog.esq", `package catalog

/// Where a product is in its life.
enum Stage { DRAFT LIVE SOLD_OUT }

entity Category {
  name string required unique max(80)
}

/// Something the shop sells.
entity Product {
  name        string required min(3) max(120)
  description text
  stock       int    default(0) min(0) max(1000)
  stage       Stage  default(DRAFT)
  /// Stock keeping unit.
  sku         string pattern(`+"`^[A-Z]{3}-\\d+$`"+`)
  launched_on date
  details     json
  category    > Category (name) required
  tags        <> Tag
}

entity Tag {
  name string required
}
`)
	write("esquadro/orders.esq", `package orders

entity Order {
  number    string required
  placed_at time   required
}

entity OrderItem {
  order    > Order required
  product  > Product required
  quantity int64 min(1)
}
`)

	if code, out, errOut := gen(t, dir); code != 0 {
		t.Fatalf("gen: exit %d\n%s%s", code, out, errOut)
	}

	goTool := func(args ...string) {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	goTool("build", "./...")
	goTool("vet", "./...")

	var goFiles []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") {
			goFiles = append(goFiles, p)
			if forbiddenNames.MatchString(filepath.Base(p)) {
				t.Errorf("generated file %s has a technical role name", p)
			}
		}
		return nil
	})
	if len(goFiles) < 10 {
		t.Fatalf("expected the whole project, got %d Go files", len(goFiles))
	}

	if _, err := exec.LookPath("gopls"); err == nil {
		cmd := exec.Command("gopls", append([]string{"check", "-severity=hint"}, goFiles...)...)
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		if len(strings.TrimSpace(string(out))) > 0 {
			t.Errorf("gopls finds problems in the generated code:\n%s", out)
		}
	}

	// Running gen again changes nothing.
	if code, out, _ := gen(t, dir); code != 0 || !strings.Contains(out, "0 created, 0 updated, 0 deleted") {
		t.Fatalf("second gen: exit %d\n%s", code, out)
	}
}
